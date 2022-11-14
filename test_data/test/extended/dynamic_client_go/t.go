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
	g.It("L2 [apigroup:a6d1.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		for gvr, _ := range gvrs {
			_ = dynamicClient.Resource(gvr)
		}
	})
})

var _ = g.Describe("gvr as a map key from another pkg", func() {
	gvrs := other_pkg.GetMapGVRKey()
	g.It("L2 [apigroup:f832.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		for gvr, _ := range gvrs {
			_ = dynamicClient.Resource(gvr)
		}
	})
})

var _ = g.Describe("dynamic client is created at Describe level [apigroup:3e90.openshift.io]", func() {
	gvr := schema.GroupVersionResource{Group: "3e90.openshift.io", Version: "v1", Resource: "testdata"}
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	res := dynamicClient.Resource(gvr)

	g.It("L2", func() {
		res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})
})

var _ = g.Describe("gvr is created directly when creating dynamic interface", func() {
	g.It("L2 [apigroup:3db4.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(schema.GroupVersionResource{Group: "3db4.openshift.io", Version: "v1", Resource: "testdata"})
	})
})

var _ = g.Describe("gvr is created as var just before creating dynamic interface", func() {
	g.It("L2 [apigroup:d2e2.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "d2e2.openshift.io", Version: "v1", Resource: "testdata"}
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})

var _ = g.Describe("gvr var is passed to a function", func() {
	g.It("L2 [apigroup:33a9.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "33a9.openshift.io", Version: "v1", Resource: "testdata"}
		doStuffWithGVR(gvr)
	})
})

var _ = g.Describe("gvr is passed to a function", func() {
	g.It("L2 [apigroup:bf3f.openshift.io]", func() {
		doStuffWithGVR(schema.GroupVersionResource{Group: "bf3f.openshift.io", Version: "v1", Resource: "testdata"})
	})
})

func doStuffWithGVR(gvr schema.GroupVersionResource) {
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	_ = dynamicClient.Resource(gvr)
}

var _ = g.Describe("gvr is created on different level", func() {
	gvr := schema.GroupVersionResource{Group: "40fd.openshift.io", Version: "v1", Resource: "testdata"}
	g.It("L2 [apigroup:40fd.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})

var (
	gvr = schema.GroupVersionResource{Group: "9080.openshift.io", Version: "v1", Resource: "testdata"}
	_   = g.Describe("gvr is created on pkg level", func() {
		g.It("L2 [apigroup:9080.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr)
		})
	})
)

var (
	gvr2 = other_pkg.GVR("883a.openshift.io", "v1", "testdata")
	_    = g.Describe("gvr is created on pkg level using helper function", func() {
		g.It("L2 [apigroup:883a.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr2)
		})
	})
)

var (
	gvrs = []schema.GroupVersionResource{
		{Group: "cf34.openshift.io", Version: "v1", Resource: "testdata"},
		{Group: "1a1b.openshift.io", Version: "v1", Resource: "testdata"},
	}
	_ = g.Describe("gvr slice on package level", func() {
		g.It("L2 [apigroup:cf34.openshift.io][apigroup:1a1b.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrs2 = []schema.GroupVersionResource{
		other_pkg.GVR("efd0.openshift.io", "v1", "testdata"),
		other_pkg.GVR("08fa.openshift.io", "v1", "testdata"),
	}
	_ = g.Describe("gvr slice on package level created with helper func", func() {
		g.It("L2 [apigroup:efd0.openshift.io][apigroup:08fa.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs2 {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrMap = map[schema.GroupVersionResource]bool{
		{Group: "105a.openshift.io", Version: "v1", Resource: "testdata"}: true,
		{Group: "57fb.openshift.io", Version: "v1", Resource: "testdata"}: true,
	}
	_ = g.Describe("gvr as map key on package level", func() {
		g.It("L2 [apigroup:105a.openshift.io][apigroup:57fb.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for gvr, _ := range gvrMap {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)
var (
	gvrMap2 = map[schema.GroupVersionResource]bool{
		other_pkg.GVR("e4ad.openshift.io", "v1", "testdata"): true,
		other_pkg.GVR("73be.openshift.io", "v1", "testdata"): true,
	}
	_ = g.Describe("gvr as map key on package level", func() {
		g.It("L2 [apigroup:e4ad.openshift.io][apigroup:73be.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for gvr, _ := range gvrMap2 {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrMapVal = map[int]schema.GroupVersionResource{
		1: {Group: "f1af.openshift.io", Version: "v1", Resource: "testdata"},
		2: {Group: "d1b8.openshift.io", Version: "v1", Resource: "testdata"},
	}
	_ = g.Describe("gvr as map key on package level", func() {
		g.It("L2 [apigroup:f1af.openshift.io][apigroup:d1b8.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrMapVal {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrMapVal2 = map[int]schema.GroupVersionResource{
		1: other_pkg.GVR("cc45.openshift.io", "v1", "testdata"),
		2: other_pkg.GVR("fff0.openshift.io", "v1", "testdata"),
	}
	_ = g.Describe("gvr as map key on package level", func() {
		g.It("L2 [apigroup:cc45.openshift.io][apigroup:fff0.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrMapVal2 {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var _ = g.Describe("gvr outside openshift.io should be ignored", func() {
	g.It("L2", func() {
		gvr := schema.GroupVersionResource{Group: "9801.k8s.io", Version: "v1", Resource: "testdata"}
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})
})
