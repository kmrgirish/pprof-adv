package pb

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
	"google.golang.org/protobuf/proto"
)

type Stack struct {
	Name     string
	FileName string
}

// AnalyzeCPUProfile analyzes a pprof profile and returns CPU usage percentage per function
// along with their call stacks. Returns a map of function nodes containing CPU percentages
// and parent-child relationships representing the complete call graph.
//
// Example:
//
//	profile := &pb.Profile{...}
//	cpuData, err := AnalyzeCPUProfile(profile)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for name, node := range cpuData {
//	    fmt.Printf("%s: %.2f%% (self), %.2f%% (total)\n", name, node.SelfCPU, node.TotalCPU)
//	}
func AnalyzeCPUProfile(p *Profile, attrCPU bool) (map[string]*FunctionNode, error) {
	if p == nil {
		return nil, fmt.Errorf("nil profile")
	}

	// Build function info map first
	funcInfoMap := buildFunctionInfoMap(p)

	// Find CPU sample type index
	cpuIdx := -1
	for i, st := range p.SampleType {
		typeName := p.StringTable[st.Type]
		if strings.Contains(strings.ToLower(typeName), "cpu") {
			cpuIdx = i
			break
		}
	}
	if cpuIdx == -1 {
		return nil, fmt.Errorf("no CPU samples found in profile")
	}

	// Calculate total CPU time
	var totalCPU int64
	for _, sample := range p.Sample {
		if len(sample.Value) > cpuIdx {
			totalCPU += sample.Value[cpuIdx]
		}
	}
	if totalCPU == 0 {
		return nil, fmt.Errorf("no CPU time recorded in profile")
	}

	// Create function call tree
	functionNodes := make(map[string]*FunctionNode)

	// Process each sample
	for _, sample := range p.Sample {
		if len(sample.Value) <= cpuIdx {
			continue
		}

		cpuTime := float64(sample.Value[cpuIdx]) / float64(totalCPU) * 100
		stack := make([]Stack, 0, len(sample.LocationId))

		// Build stack trace
		for i := len(sample.LocationId) - 1; i >= 0; i-- {
			loc := findLocation(p, sample.LocationId[i])
			if loc == nil || len(loc.Line) == 0 {
				continue
			}

			if info, exists := funcInfoMap[loc.Line[0].FunctionId]; exists {
				stack = append(stack, Stack{
					Name:     info.Name,
					FileName: info.FileName,
				})
			}
		}

		// Update function nodes with this sample
		if len(stack) > 0 {
			updateFunctionNodes(functionNodes, stack, cpuTime, attrCPU)
		}
	}

	return functionNodes, nil
}

// FunctionNode represents a node in the call tree with CPU usage information
type FunctionNode struct {
	Name        string
	FileName    string  // Added field for source file name
	SelfAttrCPU float64 // CPU time spent in this function only (excluding children and including core functions)
	SelfCPU     float64 // CPU time spent in this function only
	TotalCPU    float64 // CPU time including children
	Children    map[string]*FunctionNode
	ParentCount int // Number of times this function appears in different call stacks
}

// FunctionInfo stores the mapping of function details
type FunctionInfo struct {
	Name     string
	FileName string
}

// buildFunctionInfoMap creates a map of function IDs to their names and file names
func buildFunctionInfoMap(p *Profile) map[uint64]FunctionInfo {
	funcMap := make(map[uint64]FunctionInfo)
	for _, fn := range p.Function {
		if fn.Name < int64(len(p.StringTable)) {
			info := FunctionInfo{
				Name: p.StringTable[fn.Name],
			}
			if fn.Filename < int64(len(p.StringTable)) {
				info.FileName = p.StringTable[fn.Filename]
			}
			funcMap[fn.Id] = info
		}
	}
	return funcMap
}

// Helper function to find location by ID
func findLocation(p *Profile, id uint64) *Location {
	for _, loc := range p.Location {
		if loc.Id == id {
			return loc
		}
	}
	return nil
}

// Helper function to update function nodes with a stack sample
func updateFunctionNodes(
	nodes map[string]*FunctionNode,
	stack []Stack,
	cpuTime float64,
	attrCPU bool,
) {
	// Process each function in the stack
	for i := len(stack) - 1; i >= 0; i-- {
		entry := stack[i]
		node, exists := nodes[entry.Name]
		if !exists {
			node = &FunctionNode{
				Name:     entry.Name,
				FileName: entry.FileName,
				Children: make(map[string]*FunctionNode),
			}
			nodes[entry.Name] = node
		}

		// Update CPU times
		node.ParentCount++
		node.TotalCPU += cpuTime
		if i == len(stack)-1 { // Leaf function gets the self time
			node.SelfCPU += cpuTime
			node.SelfAttrCPU += cpuTime
		}

		if i == len(stack)-2 {
			childFuncName := stack[i+1].Name
			if attrCPU && shouldAttrFn(childFuncName) {
				node.SelfAttrCPU += cpuTime
			}
		}

		// Add child relationship - the caller (at i) is the parent of the callee (at i+1)
		if i < len(stack)-1 {
			childEntry := stack[i+1]
			if _, exists := node.Children[childEntry.Name]; !exists {
				node.Children[childEntry.Name] = nodes[childEntry.Name]
			}
		}
	}
}

func Parse(r io.Reader) (*Profile, error) {
	bytes, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	profile := &Profile{}
	err = proto.Unmarshal(bytes, profile)
	return profile, err
}

// shouldAttrFn checks if a function name is a core function (not a user-defined function)
// e.g. runtime mallocs, mapaccess, concat string, etc.
var shouldAttrFn = func() func(string) bool {
	pkgs, err := packages.Load(nil, "std")
	if err != nil {
		fmt.Printf("error loading std packages: %v\n", err)
		os.Exit(1)
	}

	return func(funcName string) bool {
		for _, pkg := range pkgs {
			if strings.HasPrefix(funcName, pkg.PkgPath+".") {
				return true
			}
		}

		return false
	}
}()
