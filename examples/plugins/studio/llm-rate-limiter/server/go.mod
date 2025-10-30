module github.com/TykTechnologies/midsommar/examples/plugins/studio/llm-rate-limiter

go 1.24

require (
	github.com/TykTechnologies/midsommar/microgateway v0.0.0
	github.com/TykTechnologies/midsommar/v2 v2.0.0
)

require (
	github.com/fatih/color v1.17.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.7.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.32.0 // indirect
	golang.org/x/text v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.68.1 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/TykTechnologies/midsommar/v2 => ../../../../../

replace github.com/TykTechnologies/midsommar/microgateway => ../../../../../microgateway
