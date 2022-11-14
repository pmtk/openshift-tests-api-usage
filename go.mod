module github.com/pmtk/openshift-tests-api-usage

go 1.18

require (
	golang.org/x/tools v0.1.12
	k8s.io/klog/v2 v2.70.1
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4 // indirect
	golang.org/x/sys v0.1.0 // indirect
)

replace github.com/pmtk/openshift-tests-api-usage/test_data => ./test_data
