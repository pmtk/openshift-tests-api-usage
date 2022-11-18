// mimics test/extended/etcd/etcd_storage_path.go

package other_pkg

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func GetGVRS() (int, []schema.GroupVersionResource) {
	i := 0
	gvr1 := schema.GroupVersionResource{Group: "jd9e.openshift.io", Version: "v1", Resource: "testdata"}
	return i, []schema.GroupVersionResource{
		gvr1,
		{Group: "9dk3.openshift.io", Version: "v1", Resource: "testdata"},
	}
}

func GetGVRS2() (int, []schema.GroupVersionResource) {
	i := 0
	gvrs := []schema.GroupVersionResource{
		{Group: "dj98.openshift.io", Version: "v1", Resource: "testdata"},
		{Group: "dl39.openshift.io", Version: "v1", Resource: "testdata"},
	}
	return i, gvrs
}

func GetMapGVRKey() map[schema.GroupVersionResource]bool {
	gvr1 := schema.GroupVersionResource{Group: "f832.openshift.io", Version: "v1", Resource: "testdata"}
	return map[schema.GroupVersionResource]bool{
		gvr1: true,
		schema.GroupVersionResource{Group: "8ad5.openshift.io", Version: "v1", Resource: "testdata"}: true,
	}
}

func getMap() map[schema.GroupVersionResource]bool {
	gvr1 := GVR("a6d1.openshift.io", "v1", "testdata")
	m := map[schema.GroupVersionResource]bool{
		gvr1: true,
		GVR("c5ab.openshift.io", "v1", "testdata"): true,
	}
	return m
}

func GetMapGVRKeyFromFunc() (map[schema.GroupVersionResource]bool, bool) {
	return getMap(), true
}

func GVR(g, v, r string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: g, Version: v, Resource: r}
}

// TODO
//func DoStuffWithGVR(gvr schema.GroupVersionResource) {
//	dynamicClient := dynamic.NewForConfigOrDie(nil)
//	gvrIndirection := gvr
//	_ = dynamicClient.Resource(gvrIndirection)
//}

// TODO: Function returning struct containing GVR
