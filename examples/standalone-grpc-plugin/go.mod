module standalone-grpc-plugin

go 1.26.6

replace github.com/TykTechnologies/midsommar/v2 => ../..

replace github.com/TykTechnologies/midsommar/microgateway => ../../microgateway

require (
	github.com/TykTechnologies/midsommar/microgateway v0.0.0-00010101000000-000000000000
	google.golang.org/grpc v1.84.0
)

require (
	github.com/TykTechnologies/midsommar/v2 v2.2.1-0.20260930035819-6301ffbeec24 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
