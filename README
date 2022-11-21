# Static analysis of tests in [openshift/origin](https://github.com/openshift/origin)

Repository contains code used to statically analyze tests and obtain list of OpenShift API Group used within that test.

Current state of the project is WIP/POC mixture. Being result of spontaneous and hurried hack-like development there's a lot of code duplicatation and TODOs. 

It currently uses only AST. Previous versions based on call graphs, SSA, or SSI are only present in commit history.

## Considered approaches

### [Abstract Syntax Tree](https://pkg.go.dev/go/ast)

Currently used approach. Quickly parses source, base for other approaches so it already contains all the information.
#### dynamic client-go

dynamic client-go is handled using AST by traversing the tree looking for Call Expressions(CallExpr). Just CallExprs calling [Resource(schema.GroupVersionResource) on dynamic.Interface](https://github.com/kubernetes/client-go/blob/v0.25.4/dynamic/interface.go#L30) are considered. From that point, passed in GVR is traced back to its creation.

### [Single static-assignment (SSA) intermediate representation (IR) form](https://pkg.go.dev/golang.org/x/tools/go/ssa)

SSA is still quite close to source code (not intended for machine code generation), created out of AST. It provides a data where function call and called function are linked, to it was easy to traverse, but only in one way, so `GroupVersionResource` var would need to be stored for later and properly matched when used.


### [Single static-information (SSI) intermediate representation (IR) form](https://pkg.go.dev/honnef.co/go/tools@v0.3.3/go/ir)

Improvement upon SSA, most notably allows for two way traversal.

### [Rapid Type Analysis (RTA) call graph](https://pkg.go.dev/golang.org/x/tools/go/callgraph/rta)

Requires creation of both AST and SSA before hand. Takes significant time to compute. Sometimes produced call graphs for some test packages resulting in unexpected cyclic graphs. Faster algorithms weren't good enough, more accurate [pointer analysis](https://pkg.go.dev/golang.org/x/tools/go/pointer) wasn't happy with origin repository structure.

## Roadmap/TODOs

- client-go
  - [ ] Create test data in `test_data/test/extended/client_go`
  - [ ] Create functionality to detect & interpret OpenShift's client-go usage
- dynamic client-go
  - [ ] Handle remaining TODOs, among which:
    - [ ] Handle usage of dynamic.Interface in free functions - this requires looking for a places where that function is called and which what GVR, and tracing back to that GVR's creation
    - [ ] Handle GVRs as struct's fields - detect and find creation
    - [ ] Investigate handling dynamic creation of GVRs ([example](https://github.com/openshift/origin/blob/master/test/extended/templates/helpers.go#L394))
  - [ ] Deduplicate and clean up code (ideally function for each ast type)
- [CLI](https://github.com/openshift/origin/blob/master/test/extended/util/client.go)
  - [ ] Create test data in `test_data/test/extended/cli`
  - [ ] Create functionality to detect & interpret CLI usage

