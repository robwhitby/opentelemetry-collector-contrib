module github.com/open-telemetry/opentelemetry-collector-contrib/receiver/chronyreceiver

go 1.18

require (
	github.com/facebook/time v0.0.0-20220713225404-f7a0d7702d50
	github.com/stretchr/testify v1.8.1
	github.com/tilinna/clock v1.1.0
	go.opentelemetry.io/collector v0.69.2-0.20230120182939-d97f1eb5bf86
	go.opentelemetry.io/collector/component v0.69.2-0.20230120182939-d97f1eb5bf86
	go.opentelemetry.io/collector/confmap v0.69.2-0.20230120182939-d97f1eb5bf86
	go.opentelemetry.io/collector/consumer v0.69.2-0.20230120182939-d97f1eb5bf86
	go.opentelemetry.io/collector/pdata v1.0.0-rc3.0.20230120182939-d97f1eb5bf86
	go.uber.org/multierr v1.9.0
	go.uber.org/zap v1.24.0
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/knadh/koanf v1.4.5 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/mapstructure v1.5.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	go.opencensus.io v0.24.0 // indirect
	go.opentelemetry.io/collector/featuregate v0.69.2-0.20230120182939-d97f1eb5bf86 // indirect
	go.opentelemetry.io/otel v1.11.2 // indirect
	go.opentelemetry.io/otel/metric v0.34.0 // indirect
	go.opentelemetry.io/otel/trace v1.11.2 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/net v0.5.0 // indirect
	golang.org/x/sys v0.4.0 // indirect
	golang.org/x/text v0.6.0 // indirect
	google.golang.org/genproto v0.0.0-20221118155620-16455021b5e6 // indirect
	google.golang.org/grpc v1.52.0 // indirect
	google.golang.org/protobuf v1.28.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

retract v0.65.0
