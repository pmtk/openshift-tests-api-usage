package main

import (
	"fmt"
)

type Node interface {
	String() string
	AddChild(Node)
	GetParent() Node
	GetChildren() []Node
}

var _ Node = (*node)(nil)

type node struct {
	parent   Node
	children []Node
}

func (n *node) AddChild(c Node) {
	switch nn := c.(type) {
	case *GinkgoNode:
		nn.node.parent = n
	case *APIUsageNode:
		nn.node.parent = n
	case *HelperFunctionNode:
		nn.node.parent = n
	}

	n.children = append(n.children, c)
}

func (n *node) GetParent() Node {
	if n.parent == nil {
		return n
	}
	return n.parent
}
func (n *node) String() string      { return "" }
func (n *node) GetChildren() []Node { return n.children }

//////////////////////////////////////////////////

var _ Node = (*RootNode)(nil)

type RootNode struct {
	node
}

func (rn *RootNode) String() string { return "Root" }

func NewRootNode() Node { return &RootNode{} }

//////////////////////////////////////////////////

type GinkgoNodeType string

const (
	GinkgoDescribe GinkgoNodeType = "Describe"
	GinkgoIt       GinkgoNodeType = "It"
)

var _ Node = (*GinkgoNode)(nil)

type GinkgoNode struct {
	node

	Type     GinkgoNodeType
	Filepath string
	Desc     string
}

func (gn *GinkgoNode) String() string {
	return fmt.Sprintf("%s(%s) @ %s", gn.Type, gn.Desc, gn.Filepath)
}

func NewGinkgoNode(t GinkgoNodeType, file, desc string) Node {
	return &GinkgoNode{Type: t, Filepath: file, Desc: desc}
}

//////////////////////////////////////////////////

var _ Node = (*APIUsageNode)(nil)

type APIUsageNode struct {
	node
	Pkg  string
	Recv string
	Func string
	Args string
}

func NewAPIUsageNode(pkg, recv, fun string) Node {
	return &APIUsageNode{Pkg: pkg, Recv: recv, Func: fun}
}

func NewAPIUsageNodeWithArgs(pkg, recv, fun, args string) Node {
	return &APIUsageNode{Pkg: pkg, Recv: recv, Func: fun, Args: args}
}

func (a *APIUsageNode) String() string {
	if a.Recv != "" {
		return fmt.Sprintf("API (%s.%s).%s(%s)", a.Pkg, a.Recv, a.Func, a.Args)
	}
	return fmt.Sprintf("API: (%s).%s(%s)", a.Pkg, a.Func, a.Args)
}

func (a *APIUsageNode) Hash() string {
	// TODO: Handle Args if relevant (probably just for *CLI?)
	return fmt.Sprintf("%s#%s#%s", a.Pkg, a.Recv, a.Func)
}

func (a *APIUsageNode) AddChild(c Node) {
	panic("APIUsageNode is not expected to have children")
}

//////////////////////////////////////////////////

var _ Node = (*HelperFunctionNode)(nil)

type HelperFunctionNode struct {
	node
	Pkg  string
	Func string
}

func NewHelperFunctionNode(pkg, fun string) Node {
	// Current assumption that helper function is not a method might be wrong (most likely for upgrade tests)
	// TODO: Handle helper methods if needed
	return &HelperFunctionNode{Pkg: pkg, Func: fun}
}

func (a *HelperFunctionNode) String() string {
	return fmt.Sprintf("Helper: (%s).%s()", a.Pkg, a.Func)
}

func (a *HelperFunctionNode) Hash() string {
	return fmt.Sprintf("%s#%s", a.Pkg, a.Func)
}

//////////////////////////////////////////////////

func nodeAndChildString(n Node, level int) string {
	out := n.String()
	for _, child := range n.GetChildren() {
		out += "\n"
		for i := 0; i < level; i++ {
			out += "  "
		}
		out += nodeAndChildString(child, level+1)
	}
	return out
}

func printTree(root Node) {
	fmt.Printf("%s\n", nodeAndChildString(root, 1))
}
