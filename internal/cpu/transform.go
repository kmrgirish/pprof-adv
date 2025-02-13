package cpu

import (
	"fmt"
	"io"

	"github.com/kmrgirish/pprof-adv/pb"
)

// Transform converts the pprof format into a raw text format where real cpu% usages is attributed to a function instead of it's childs
func Transform(pprof *pb.Profile, w io.Writer, attrCPU bool) error {
	profile, err := pb.AnalyzeCPUProfile(pprof, attrCPU)
	if err != nil {
		return err
	}

	for fn, node := range profile {
		fmt.Fprintf(w, "%.2f\t%s in %s\n", node.SelfAttrCPU, fn, node.FileName)
	}

	return nil
}
