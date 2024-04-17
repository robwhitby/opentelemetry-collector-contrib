// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package githubactionsreceiver

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/google/go-github/v61/github"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.uber.org/zap"
)

func eventToTraces(event interface{}, config *Config, logger *zap.Logger) (ptrace.Traces, error) {
	logger.Debug("Determining event")
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	switch e := event.(type) {
	case *github.WorkflowJobEvent:
		logger.Info("Processing WorkflowJobEvent", zap.String("job_name", e.WorkflowJob.GetName()), zap.String("repo", e.Repo.GetFullName()))
		jobResource := resourceSpans.Resource()
		createResourceAttributes(jobResource, e, config, logger)

		traceID, err := generateTraceID(e.WorkflowJob.GetRunID(), e.WorkflowJob.GetRunAttempt())
		if err != nil {
			logger.Error("Failed to generate trace ID", zap.Error(err))
			return ptrace.Traces{}, fmt.Errorf("failed to generate trace ID: %w", err)
		}

		if e.WorkflowJob.GetStatus() == "completed" {
			parentSpanID := createParentSpan(scopeSpans, e.WorkflowJob.Steps, e.WorkflowJob, traceID, logger)
			processSteps(scopeSpans, e.WorkflowJob.Steps, e.WorkflowJob, traceID, parentSpanID, logger)
		}

	case *github.WorkflowRunEvent:
		logger.Info("Processing WorkflowRunEvent", zap.String("workflow_name", e.WorkflowRun.GetName()), zap.String("repo", e.Repo.GetFullName()))
		runResource := resourceSpans.Resource()

		traceID, err := generateTraceID(e.WorkflowRun.GetID(), int64(e.WorkflowRun.GetRunAttempt()))
		if err != nil {
			logger.Error("Failed to generate trace ID", zap.Error(err))
			return ptrace.Traces{}, fmt.Errorf("failed to generate trace ID: %w", err)
		}

		if e.WorkflowRun.GetStatus() == "completed" {
			createResourceAttributes(runResource, e, config, logger)
			createRootSpan(resourceSpans, e, traceID, logger)
		}

	default:
		logger.Error("unknown event type, dropping payload")
		return ptrace.Traces{}, fmt.Errorf("unknown event type")
	}

	return traces, nil
}

func createParentSpan(scopeSpans ptrace.ScopeSpans, steps []*github.TaskStep, job *github.WorkflowJob, traceID pcommon.TraceID, logger *zap.Logger) pcommon.SpanID {
	logger.Debug("Creating parent span", zap.String("name", job.GetName()))
	span := scopeSpans.Spans().AppendEmpty()
	span.SetTraceID(traceID)

	parentSpanID, _ := generateParentSpanID(job.GetRunID(), job.GetRunAttempt())
	span.SetParentSpanID(parentSpanID)

	jobSpanID, _ := generateJobSpanID(job.GetRunID(), job.GetRunAttempt(), job.GetName())
	span.SetSpanID(jobSpanID)

	span.SetName(job.GetName())
	span.SetKind(ptrace.SpanKindServer)
	if len(steps) > 0 {
		setSpanTimes(span, steps[0].GetStartedAt(), steps[len(steps)-1].GetCompletedAt())
	} else {
		// Invoked when status skipped or cancelled
		logger.Warn("No steps found, defaulting to job times")
		setSpanTimes(span, job.GetStartedAt(), job.GetCompletedAt())
	}

	allSuccessful := true
	anyFailure := false
	for _, step := range steps {
		if step.GetStatus() != "completed" || step.GetConclusion() != "success" {
			allSuccessful = false
		}
		if step.GetConclusion() == "failure" {
			anyFailure = true
			break
		}
	}

	if anyFailure {
		span.Status().SetCode(ptrace.StatusCodeError)
	} else if allSuccessful {
		span.Status().SetCode(ptrace.StatusCodeOk)
	} else {
		span.Status().SetCode(ptrace.StatusCodeUnset)
	}

	span.Status().SetMessage(job.GetConclusion())

	return span.SpanID()
}

func checkDuplicateStepNames(steps []*github.TaskStep) map[string]int {
	nameCount := make(map[string]int)
	for _, step := range steps {
		nameCount[step.GetName()]++
	}
	return nameCount
}

func convertPRURL(apiURL string) string {
	apiURL = strings.Replace(apiURL, "/repos", "", 1)
	apiURL = strings.Replace(apiURL, "/pulls", "/pull", 1)
	return strings.Replace(apiURL, "api.", "", 1)
}

func createRootSpan(resourceSpans ptrace.ResourceSpans, event *github.WorkflowRunEvent, traceID pcommon.TraceID, logger *zap.Logger) (pcommon.SpanID, error) {
	logger.Debug("Creating root parent span", zap.String("name", event.WorkflowRun.GetName()))
	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()
	span := scopeSpans.Spans().AppendEmpty()

	rootSpanID, err := generateParentSpanID(event.WorkflowRun.GetID(), int64(event.WorkflowRun.GetRunAttempt()))
	if err != nil {
		logger.Error("Failed to generate root span ID", zap.Error(err))
		return pcommon.SpanID{}, fmt.Errorf("failed to generate root span ID: %w", err)
	}

	span.SetTraceID(traceID)
	span.SetSpanID(rootSpanID)
	span.SetName(event.WorkflowRun.GetName())
	span.SetKind(ptrace.SpanKindServer)
	setSpanTimes(span, event.WorkflowRun.GetRunStartedAt(), event.WorkflowRun.GetUpdatedAt())

	switch event.WorkflowRun.GetConclusion() {
	case "success":
		span.Status().SetCode(ptrace.StatusCodeOk)
	case "failure":
		span.Status().SetCode(ptrace.StatusCodeError)
	default:
		span.Status().SetCode(ptrace.StatusCodeUnset)
	}

	span.Status().SetMessage(event.WorkflowRun.GetConclusion())

	// Attempt to link to previous trace ID if applicable
	if event.WorkflowRun.GetPreviousAttemptURL() != "" && event.WorkflowRun.GetRunAttempt() > 1 {
		logger.Debug("Linking to previous trace ID for WorkflowRunEvent")
		previousRunAttempt := event.WorkflowRun.GetRunAttempt() - 1
		previousTraceID, err := generateTraceID(event.WorkflowRun.GetID(), int64(previousRunAttempt))
		if err != nil {
			logger.Error("Failed to generate previous trace ID", zap.Error(err))
		} else {
			link := span.Links().AppendEmpty()
			link.SetTraceID(previousTraceID)
			logger.Debug("Successfully linked to previous trace ID", zap.String("previousTraceID", previousTraceID.String()))
		}
	}

	return rootSpanID, nil
}

func createSpan(scopeSpans ptrace.ScopeSpans, step *github.TaskStep, job *github.WorkflowJob, traceID pcommon.TraceID, parentSpanID pcommon.SpanID, logger *zap.Logger, stepNumber ...int) pcommon.SpanID {
	logger.Debug("Processing span", zap.String("step_name", step.GetName()))
	span := scopeSpans.Spans().AppendEmpty()
	span.SetTraceID(traceID)
	span.SetParentSpanID(parentSpanID)

	var spanID pcommon.SpanID

	span.Attributes().PutStr("ci.github.workflow.job.step.name", step.GetName())
	span.Attributes().PutStr("ci.github.workflow.job.step.status", step.GetStatus())
	span.Attributes().PutStr("ci.github.workflow.job.step.conclusion", step.GetConclusion())
	if len(stepNumber) > 0 && stepNumber[0] > 0 {
		spanID, _ = generateStepSpanID(job.GetRunID(), job.GetRunAttempt(), job.GetName(), step.GetName(), stepNumber[0])
		span.Attributes().PutInt("ci.github.workflow.job.step.number", int64(stepNumber[0]))
	} else {
		spanID, _ = generateStepSpanID(job.GetRunID(), job.GetRunAttempt(), job.GetName(), step.GetName())
		span.Attributes().PutInt("ci.github.workflow.job.step.number", step.GetNumber())
	}
	span.Attributes().PutStr("ci.github.workflow.job.step.started_at", step.GetStartedAt().Format(time.RFC3339))
	span.Attributes().PutStr("ci.github.workflow.job.step.completed_at", step.GetCompletedAt().Format(time.RFC3339))

	span.SetSpanID(spanID)

	setSpanTimes(span, step.GetStartedAt(), step.GetCompletedAt())
	span.SetName(step.GetName())
	span.SetKind(ptrace.SpanKindServer)

	switch step.GetConclusion() {
	case "success":
		span.Status().SetCode(ptrace.StatusCodeOk)
	case "failure":
		span.Status().SetCode(ptrace.StatusCodeError)
	default:
		span.Status().SetCode(ptrace.StatusCodeUnset)
	}

	span.Status().SetMessage(step.GetConclusion())

	return span.SpanID()
}

func generateTraceID(runID int64, runAttempt int64) (pcommon.TraceID, error) {
	input := fmt.Sprintf("%d%dt", runID, runAttempt)
	hash := sha256.Sum256([]byte(input))
	traceIDHex := hex.EncodeToString(hash[:])

	var traceID pcommon.TraceID
	_, err := hex.Decode(traceID[:], []byte(traceIDHex[:32]))
	if err != nil {
		return pcommon.TraceID{}, err
	}

	return traceID, nil
}

func generateJobSpanID(runID int64, runAttempt int64, job string) (pcommon.SpanID, error) {
	input := fmt.Sprintf("%d%d%s", runID, runAttempt, job)
	hash := sha256.Sum256([]byte(input))
	spanIDHex := hex.EncodeToString(hash[:])

	var spanID pcommon.SpanID
	_, err := hex.Decode(spanID[:], []byte(spanIDHex[16:32]))
	if err != nil {
		return pcommon.SpanID{}, err
	}

	return spanID, nil
}

func generateParentSpanID(runID int64, runAttempt int64) (pcommon.SpanID, error) {
	input := fmt.Sprintf("%d%ds", runID, runAttempt)
	hash := sha256.Sum256([]byte(input))
	spanIDHex := hex.EncodeToString(hash[:])

	var spanID pcommon.SpanID
	_, err := hex.Decode(spanID[:], []byte(spanIDHex[16:32]))
	if err != nil {
		return pcommon.SpanID{}, err
	}

	return spanID, nil
}

func generateServiceName(config *Config, fullName string) string {
	if config.CustomServiceName != "" {
		return config.CustomServiceName
	}
	formattedName := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(fullName, "/", "-"), "_", "-"))
	return fmt.Sprintf("%s%s%s", config.ServiceNamePrefix, formattedName, config.ServiceNameSuffix)
}

func generateStepSpanID(runID int64, runAttempt int64, jobName, stepName string, stepNumber ...int) (pcommon.SpanID, error) {
	var input string
	if len(stepNumber) > 0 && stepNumber[0] > 0 {
		input = fmt.Sprintf("%d%d%s%s%d", runID, runAttempt, jobName, stepName, stepNumber[0])
	} else {
		input = fmt.Sprintf("%d%d%s%s", runID, runAttempt, jobName, stepName)
	}
	hash := sha256.Sum256([]byte(input))
	spanIDHex := hex.EncodeToString(hash[:])

	var spanID pcommon.SpanID
	_, err := hex.Decode(spanID[:], []byte(spanIDHex[16:32]))
	if err != nil {
		return pcommon.SpanID{}, err
	}

	return spanID, nil
}

func processSteps(scopeSpans ptrace.ScopeSpans, steps []*github.TaskStep, job *github.WorkflowJob, traceID pcommon.TraceID, parentSpanID pcommon.SpanID, logger *zap.Logger) {
	nameCount := checkDuplicateStepNames(steps)
	for index, step := range steps {
		if nameCount[step.GetName()] > 1 {
			createSpan(scopeSpans, step, job, traceID, parentSpanID, logger, index+1) // Pass step number if duplicate names exist
		} else {
			createSpan(scopeSpans, step, job, traceID, parentSpanID, logger)
		}
	}
}

func setSpanTimes(span ptrace.Span, start, end github.Timestamp) {
	span.SetStartTimestamp(pcommon.NewTimestampFromTime(start.Time))
	span.SetEndTimestamp(pcommon.NewTimestampFromTime(end.Time))
}

func transformGitHubAPIURL(apiURL string) string {
	htmlURL := strings.Replace(apiURL, "api.github.com/repos", "github.com", 1)
	return htmlURL
}

func validateSignatureSHA256(secret string, signatureHeader string, body []byte, logger *zap.Logger) bool {
	if signatureHeader == "" || len(signatureHeader) < 7 {
		logger.Debug("Unauthorized - No Signature Header")
		return false
	}
	receivedSig := signatureHeader[7:]
	computedHash := hmac.New(sha256.New, []byte(secret))
	computedHash.Write(body)
	expectedSig := hex.EncodeToString(computedHash.Sum(nil))

	logger.Debug("Debugging Signatures", zap.String("Received", receivedSig), zap.String("Computed", expectedSig))

	return hmac.Equal([]byte(expectedSig), []byte(receivedSig))
}

func validateSignatureSHA1(secret string, signatureHeader string, body []byte, logger *zap.Logger) bool {
	if signatureHeader == "" {
		logger.Debug("Unauthorized - No Signature Header")
		return false
	}
	receivedSig := signatureHeader[5:] // Assume "sha1=" prefix
	computedHash := hmac.New(sha1.New, []byte(secret))
	computedHash.Write(body)
	expectedSig := hex.EncodeToString(computedHash.Sum(nil))

	logger.Debug("Debugging Signatures", zap.String("Received", receivedSig), zap.String("Computed", expectedSig))

	return hmac.Equal([]byte(expectedSig), []byte(receivedSig))
}
