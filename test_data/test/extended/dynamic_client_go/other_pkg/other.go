// mimics test/extended/etcd/etcd_storage_path.go

package other_pkg

import "k8s.io/apimachinery/pkg/runtime/schema"

func GetMapGVRKey() map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{
		schema.GroupVersionResource{Group: "authorization.openshift.io", Version: "v1", Resource: "roles"}: true,
	}
}

func GetMapGVRKeyFromFunc() map[schema.GroupVersionResource]bool {
	return map[schema.GroupVersionResource]bool{
		GVR("authorization.openshift.io", "v1", "roles"): true,
	}
}
func GVR(g, v, r string) schema.GroupVersionResource {
	return schema.GroupVersionResource{Group: g, Version: v, Resource: r}
}
