package dynamic_client_go

import (
	"context"

	g "github.com/onsi/ginkgo/v2"

	"github.com/pmtk/openshift-tests-api-usage/test_data/test/extended/dynamic_client_go/other_pkg"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var _ = g.Describe("ResourceInterface is created directly without DynamicConfig var, GVR is initialized literate", func() {
	g.It("field keys (ids) are used with literate strings [apigroup:23d0.openshift.io]", func() {
		res := dynamic.NewForConfigOrDie(nil).Resource(schema.GroupVersionResource{Group: "23d0.openshift.io", Version: "v1", Resource: "testdata"})
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})

	g.It("field keys are used, but values are vars [apigroup:213j.openshift.io]", func() {
		gr := "213j.openshift.io"
		v := "v1"
		r := "testdata"
		res := dynamic.NewForConfigOrDie(nil).Resource(schema.GroupVersionResource{Group: gr, Version: v, Resource: r})
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})

	g.It("field keys are used, but values are vars with reassignment [apigroup:jk34.openshift.io]", func() {
		g1 := "jk34.openshift.io"
		g2 := g1
		gr := g2
		v := "v1"
		r := "testdata"
		res := dynamic.NewForConfigOrDie(nil).Resource(schema.GroupVersionResource{Group: gr, Version: v, Resource: r})
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})

	g.It("field keys are used, but values are vars with many more reassignments [apigroup:1ew3.openshift.io]", func() {
		g1 := "1ew3.openshift.io"
		g2 := g1
		g3 := g2
		g4 := g3
		g5 := g4
		g6 := g5
		g7 := g6
		gr := g7
		v := "v1"
		r := "testdata"
		res := dynamic.NewForConfigOrDie(nil).Resource(schema.GroupVersionResource{Group: gr, Version: v, Resource: r})
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})

	g.It("field keys are unused, values are literate strings [apigroup:2b1f.openshift.io]", func() {
		res := dynamic.NewForConfigOrDie(nil).Resource(schema.GroupVersionResource{"2b1f.openshift.io", "v1", "testdata"})
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})
})

var _ = g.Describe("ResourceInterface is created from DynamicConfig object, GVR is initialized literate", func() {
	g.It("field keys are unused, values are literate strings [apigroup:3db4.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(schema.GroupVersionResource{Group: "3db4.openshift.io", Version: "v1", Resource: "testdata"})
	})

	g.It("gvr is a var [apigroup:d2e2.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "d2e2.openshift.io", Version: "v1", Resource: "testdata"}
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr)
	})

	gvr1 := schema.GroupVersionResource{Group: "40fd.openshift.io", Version: "v1", Resource: "testdata"}
	g.It("gvr is created on different level [apigroup:40fd.openshift.io]", func() {
		dynamicClient := dynamic.NewForConfigOrDie(nil)
		_ = dynamicClient.Resource(gvr1)
	})

	g.It("gvr var is passed to a function [apigroup:33a9.openshift.io]", func() {
		gvr := schema.GroupVersionResource{Group: "33a9.openshift.io", Version: "v1", Resource: "testdata"}
		doStuffWithGVR(gvr)
	})
})

var _ = g.Describe("dynamic client is created at Describe level [apigroup:3e90.openshift.io]", func() {
	gvr := schema.GroupVersionResource{Group: "3e90.openshift.io", Version: "v1", Resource: "testdata"}
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	res := dynamicClient.Resource(gvr)

	g.It("L2", func() {
		_, _ = res.Get(context.TODO(), "test-123", metav1.GetOptions{})
	})
})

var (
	gvr1 = schema.GroupVersionResource{Group: "9080.openshift.io", Version: "v1", Resource: "testdata"}
	gvr2 = other_pkg.GVR("883a.openshift.io", "v1", "testdata")
	gr   = "dnlf.openshift.io"
	gvr3 = localGVR(gr, "v1", "testdata")

	_ = g.Describe("gvr is created on pkg level", func() {
		g.It("as a literate [apigroup:9080.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr1)
		})

		g.It("using helper function from another pkg [apigroup:883a.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr2)
		})

		g.It("using local helper function with var [apigroup:dnlf.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			_ = dynamicClient.Resource(gvr3)
		})
	})
)

var (
	gvrs1 = []schema.GroupVersionResource{
		{Group: "cf34.openshift.io", Version: "v1", Resource: "testdata"},
		{Group: "1a1b.openshift.io", Version: "v1", Resource: "testdata"},
		other_pkg.GVR("efd0.openshift.io", "v1", "testdata"),
		localGVR("08fa.openshift.io", "v1", "testdata"),
	}
	_ = g.Describe("gvr slice on package level ", func() {
		g.It("literate GVRs and created with helpers (local and other pkg) [apigroup:cf34.openshift.io][apigroup:1a1b.openshift.io][apigroup:efd0.openshift.io][apigroup:08fa.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs1 {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})

	_ = g.Describe("gvr is a slice from other func", func() {
		g.It("[apigroup:jd9e.openshift.io][apigroup:9dk3.openshift.io]", func() {
			_, gvrs := other_pkg.GetGVRS()
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})

	_ = g.Describe("gvr is a slice from other func", func() {
		g.It("[apigroup:dj98.openshift.io][apigroup:dl39.openshift.io]", func() {
			_, gvrs := other_pkg.GetGVRS2()
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrs {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrMap1 = map[schema.GroupVersionResource]bool{
		{Group: "105a.openshift.io", Version: "v1", Resource: "testdata"}: true,
		{Group: "57fb.openshift.io", Version: "v1", Resource: "testdata"}: true,
		other_pkg.GVR("e4ad.openshift.io", "v1", "testdata"):              true,
		other_pkg.GVR("73be.openshift.io", "v1", "testdata"):              true,
	}
	gvrMap2, _ = other_pkg.GetMapGVRKeyFromFunc()
	gvrMap3    = other_pkg.GetMapGVRKey()

	_ = g.Describe("gvr as map key on package level", func() {
		g.It("map is created from literate structs [apigroup:105a.openshift.io][apigroup:57fb.openshift.io][apigroup:e4ad.openshift.io][apigroup:73be.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for gvr, _ := range gvrMap1 {
				_ = dynamicClient.Resource(gvr)
			}
		})

		g.It("function directly returns map with GVR literals [apigroup:a6d1.openshift.io][apigroup:c5ab.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for gvr, _ := range gvrMap2 {
				_ = dynamicClient.Resource(gvr)
			}
		})

		g.It("function returns a map but GVRs are constructed using helper function [apigroup:f832.openshift.io][apigroup:8ad5.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for gvr, _ := range gvrMap3 {
				_ = dynamicClient.Resource(gvr)
			}
		})
	})
)

var (
	gvrMapVal = map[int]schema.GroupVersionResource{
		1: {Group: "f1af.openshift.io", Version: "v1", Resource: "testdata"},
		2: {Group: "d1b8.openshift.io", Version: "v1", Resource: "testdata"},
		3: other_pkg.GVR("cc45.openshift.io", "v1", "testdata"),
		4: other_pkg.GVR("fff0.openshift.io", "v1", "testdata"),
	}
	_ = g.Describe("map[*]GVR on package level", func() {
		g.It("GVRs are created as literate structs [apigroup:f1af.openshift.io][apigroup:d1b8.openshift.io][apigroup:cc45.openshift.io][apigroup:fff0.openshift.io]", func() {
			dynamicClient := dynamic.NewForConfigOrDie(nil)
			for _, gvr := range gvrMapVal {
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

func doStuffWithGVR(gvr schema.GroupVersionResource) {
	dynamicClient := dynamic.NewForConfigOrDie(nil)
	_ = dynamicClient.Resource(gvr)
}

func localGVR(g, v, r string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: g, Version: v, Resource: r}
}
