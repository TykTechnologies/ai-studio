module github.com/TykTechnologies/midsommar/examples/plugins/custom-auth-ui/server

go 1.24.4

toolchain go1.24.9

require github.com/TykTechnologies/midsommar/v2 v2.0.0

require (
	github.com/TykTechnologies/midsommar/microgateway v0.0.0 // indirect
	github.com/fatih/color v1.17.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/hashicorp/go-hclog v1.6.3 // indirect
	github.com/hashicorp/go-plugin v1.7.0 // indirect
	github.com/hashicorp/yamux v0.1.2 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/oklog/run v1.1.0 // indirect
	github.com/rs/zerolog v1.34.0 // indirect
	github.com/simonfxr/pubsub v0.0.5 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250728155136-f173205681a0 // indirect
	google.golang.org/grpc v1.74.2 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

replace github.com/TykTechnologies/midsommar/v2 => ../../../../../

replace github.com/TykTechnologies/midsommar/microgateway => ../../../../../microgateway
