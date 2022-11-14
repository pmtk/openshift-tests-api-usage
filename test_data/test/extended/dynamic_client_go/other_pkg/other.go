// mimics test/extended/etcd/etcd_storage_path.go

package other_pkg

import "k8s.io/apimachinery/pkg/runtime/schema"

func GetMapGVRKey() map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{
		schema.GroupVersionResource{Group: "f832.openshift.io", Version: "v1", Resource: "testdata"}: true,
	}
}

func GetMapGVRKeyFromFunc() map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{
		GVR("a6d1.openshift.io", "v1", "testdata"): true,
	}
}
func GVR(g, v, r string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: g, Version: v, Resource: r}
}
