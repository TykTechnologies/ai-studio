// Checks which of a host's module versions AI Studio would raise (see
// main.go). A module of its own, so golang.org/x/mod is not a requirement
// of the root module.
module github.com/TykTechnologies/midsommar/v2/tools/hostcompat

go 1.26.6

require golang.org/x/mod v0.30.0
