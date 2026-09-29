// Checks that the root module's zip is valid (see main.go). A module of its
// own, so golang.org/x/mod is not a requirement of the root module.
module github.com/TykTechnologies/midsommar/v2/tools/modcheck

go 1.26.6

require golang.org/x/mod v0.30.0
