package dynamic_client_go

import (
	"context"
	g "github.com/onsi/ginkgo/v2"
	"github.com/pmtk/openshift-tests-api-usage/test_data/test/extended/dynamic_client_go/other_pkg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/client-go/dynamic"
)

var _ = g.Describe("gvr as a map key created via func from another pkg", func() {
	gvrs := other_pkg.GetMapGVRKeyFromFunc()
	g.It("L2 [apigroup:authorization.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		for gvr, _ := range gvrs {
			_ = dynamicClient.Resource(gvr)
		}
	})
})

var _ = g.Describe("gvr as a map key from another pkg", func() {
	gvrs := other_pkg.GetMapGVRKey()
	g.It("L2 [apigroup:authorization.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		for gvr, _ := range gvrs {
			_ = dynamicClient.Resource(gvr)
		}
	})
})

var _ = g.Describe("dynamic client is created at Describe level [apigroup:test.openshift.io]", func() {
	gvr := schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"}
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	res := dynamicClient.Resource(gvr)

	g.It("L2", func() {
		res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})
})

var _ = g.Describe("gvr is created directly when creating dynamic interface", func() {
	g.It("L2 [apigroup:test.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"})
	})
})

var _ = g.Describe("gvr is created as var just before creating dynamic interface", func() {
	g.It("L2 [apigroup:test.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"}
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})

var _ = g.Describe("gvr var is passed to a function", func() {
	g.It("L2 [apigroup:test.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"}
		doStuffWithGVR(gvr)
	})
})

var _ = g.Describe("gvr is passed to a function", func() {
	g.It("L2 [apigroup:test.openshift.io]", func() {
		doStuffWithGVR(schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"})
	})
})

func doStuffWithGVR(gvr schema.GroupVersionResource) {
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	_ = dynamicClient.Resource(gvr)
}

var _ = g.Describe("gvr is created on different level", func() {
	gvr := schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"}
	g.It("L2 [apigroup:test.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})

var (
	gvr = schema.GroupVersionResource{Group: "test.openshift.io", Version: "v1", Resource: "testdata"}
	_   = g.Describe("gvr is created on pkg level", func() {
		g.It("L2 [apigroup:test.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr)
		})
	})
)

var (
	gvr2 = other_pkg.GVR("test.openshift.io", "v1", "none")
	_    = g.Describe("gvr is created on pkg level using helper function", func() {
		g.It("L2 [apigroup:test.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr2)
		})
	})
)

var (
	gvrs = []schema.GroupVersionResource{
		{Group: "test.openshift.io", Version: "v1", Resource: "testdata"},
		{Group: "test2.openshift.io", Version: "v1", Resource: "testdata"},
	}
	_ = g.Describe("gvr slice on package level", func() {
		g.It("L2 [apigroup:test.openshift.io][apigroup:test2.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var _ = g.Describe("gvr outside openshift.io should be ignored", func() {
	g.It("L2", func() {
		gvr := schema.GroupVersionResource{Group: "test.k8s.io", Version: "v1", Resource: "testdata"}
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})
