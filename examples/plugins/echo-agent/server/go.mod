module github.com/TykTechnologies/midsommar/examples/plugins/echo-agent

go 1.23

require (
	github.com/TykTechnologies/midsommar/microgateway v0.0.0
	google.golang.org/grpc v1.69.4
)

replace github.com/TykTechnologies/midsommar/microgateway => ../../../../microgateway
