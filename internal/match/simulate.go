package match

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"lo0.ai/lo0/internal/machine"
)

// HardwareQuery names hardware to assess instead of this machine. Sizes are
// in GiB.
type HardwareQuery struct {
	// GPU names a graphics card, such as "RTX 4090".
	GPU string
	// VRAM is each card's memory, for cards sold in several sizes.
	VRAM int
	// GPUs is how many of the card there are. Zero means one.
	GPUs int
	// RAM is the system memory beside the cards. Zero keeps this machine's.
	RAM int
	// Mac names an Apple chip, such as "M4 Max" or "M4 Max 32-core".
	Mac string
	// Memory is the Mac's unified memory.
	Memory int
}

// IsZero reports whether q names no hardware.
func (q HardwareQuery) IsZero() bool { return q == HardwareQuery{} }

// maxSimulatedGPUs bounds --gpus to machines people build.
const maxSimulatedGPUs = 8

// Simulation is resolved hardware, ready to replace a scanned profile.
type Simulation struct {
	gpu   machine.GPU
	count int
	// memory is in bytes; zero keeps the scanned machine's.
	memory uint64
}

// Simulate resolves q against lo0's tables, refusing unknown names with the
// closest known ones.
func Simulate(q HardwareQuery) (Simulation, error) {
	switch {
	case q.GPU != "" && q.Mac != "":
		return Simulation{}, errors.New("name a --gpu or a --mac, not both")
	case q.GPU == "" && q.Mac == "":
		return Simulation{}, errors.New("--vram, --gpus, --ram and --memory describe hardware named with --gpu or --mac")
	case q.VRAM < 0 || q.GPUs < 0 || q.RAM < 0 || q.Memory < 0:
		return Simulation{}, errors.New("sizes and counts cannot be negative")
	case q.GPUs > maxSimulatedGPUs:
		return Simulation{}, fmt.Errorf("lo0 simulates at most %d cards", maxSimulatedGPUs)
	}
	if q.Mac != "" {
		if q.VRAM != 0 || q.GPUs != 0 || q.RAM != 0 {
			return Simulation{}, errors.New("--vram, --gpus and --ram describe a --gpu; give a Mac's memory with --memory")
		}
		return simulateMac(q.Mac, q.Memory)
	}
	if q.Memory != 0 {
		return Simulation{}, errors.New("--memory is a Mac's unified memory; give system memory beside a --gpu with --ram")
	}
	c, err := findCard(q.GPU)
	if err != nil {
		return Simulation{}, err
	}
	vram := q.VRAM
	if vram == 0 {
		if len(c.vram) > 1 {
			return Simulation{}, fmt.Errorf("the %s comes with %s GB; say which with --vram", c.short(), orList(c.vram))
		}
		vram = c.vram[0]
	}
	api := "CUDA"
	if c.vendor == machine.VendorAMD {
		api = "ROCm"
	}
	return Simulation{
		gpu:    machine.GPU{Name: c.name, Vendor: c.vendor, VRAMBytes: uint64(vram) << 30, API: api},
		count:  max(q.GPUs, 1),
		memory: uint64(q.RAM) << 30,
	}, nil
}

// Apply returns real with its hardware replaced and marked simulated. A card
// on a Mac makes it a Linux PC; the disk stays real.
func (s Simulation) Apply(real machine.Profile) machine.Profile {
	p := machine.Profile{OS: real.OS, CPU: real.CPU, Disk: real.Disk, Simulated: true}
	if s.gpu.Unified {
		p.OS = machine.OS{GOOS: "darwin", Arch: "arm64"}
		p.CPU = machine.CPU{Model: s.gpu.Name}
		p.Memory = machine.Memory{TotalBytes: s.memory, Unified: true}
		p.GPUs = []machine.GPU{s.gpu}
		return p
	}
	if real.Memory.Unified || real.OS.GOOS == "darwin" {
		p.OS = machine.OS{GOOS: "linux", Arch: "amd64"}
		p.CPU = machine.CPU{}
	}
	p.Memory = machine.Memory{TotalBytes: cmp.Or(s.memory, real.Memory.TotalBytes)}
	for range s.count {
		p.GPUs = append(p.GPUs, s.gpu)
	}
	return p
}

// card is a graphics card, named as its driver reports it.
type card struct {
	name   string
	vendor machine.Vendor
	// vram lists the sizes, in GB, the card is sold with.
	vram []int
}

// short is the card's name without brand words.
func (c card) short() string {
	s := c.name
	for _, w := range []string{"NVIDIA ", "GeForce ", "AMD ", "Radeon "} {
		s = strings.Replace(s, w, "", 1)
	}
	return s
}

// cards are the cards lo0 has figures for, with the sizes they are sold in.
var cards = []card{
	{"NVIDIA RTX PRO 6000 Blackwell Workstation Edition", machine.VendorNVIDIA, []int{96}},
	{"NVIDIA RTX 6000 Ada Generation", machine.VendorNVIDIA, []int{48}},
	{"NVIDIA RTX A6000", machine.VendorNVIDIA, []int{48}},
	{"NVIDIA H200", machine.VendorNVIDIA, []int{141}},
	{"NVIDIA H100 SXM", machine.VendorNVIDIA, []int{80}},
	{"NVIDIA H100 PCIe", machine.VendorNVIDIA, []int{80}},
	{"NVIDIA H100 NVL", machine.VendorNVIDIA, []int{94}},
	{"NVIDIA A100-SXM4-80GB", machine.VendorNVIDIA, []int{80}},
	{"NVIDIA A100-SXM4-40GB", machine.VendorNVIDIA, []int{40}},
	{"NVIDIA A100 80GB PCIe", machine.VendorNVIDIA, []int{80}},
	{"NVIDIA A100-PCIE-40GB", machine.VendorNVIDIA, []int{40}},
	{"NVIDIA L40S", machine.VendorNVIDIA, []int{48}},
	{"NVIDIA GeForce RTX 5090", machine.VendorNVIDIA, []int{32}},
	{"NVIDIA GeForce RTX 5090 Laptop GPU", machine.VendorNVIDIA, []int{24}},
	{"NVIDIA GeForce RTX 5080", machine.VendorNVIDIA, []int{16}},
	{"NVIDIA GeForce RTX 5070 Ti", machine.VendorNVIDIA, []int{16}},
	{"NVIDIA GeForce RTX 5070", machine.VendorNVIDIA, []int{12}},
	{"NVIDIA GeForce RTX 5060 Ti", machine.VendorNVIDIA, []int{8, 16}},
	{"NVIDIA GeForce RTX 4090", machine.VendorNVIDIA, []int{24}},
	{"NVIDIA GeForce RTX 4090 D", machine.VendorNVIDIA, []int{24}},
	{"NVIDIA GeForce RTX 4080 SUPER", machine.VendorNVIDIA, []int{16}},
	{"NVIDIA GeForce RTX 4080", machine.VendorNVIDIA, []int{16}},
	{"NVIDIA GeForce RTX 4070 Ti SUPER", machine.VendorNVIDIA, []int{16}},
	{"NVIDIA GeForce RTX 4070 Ti", machine.VendorNVIDIA, []int{12}},
	{"NVIDIA GeForce RTX 4070 SUPER", machine.VendorNVIDIA, []int{12}},
	{"NVIDIA GeForce RTX 4070", machine.VendorNVIDIA, []int{12}},
	{"NVIDIA GeForce RTX 4060 Ti", machine.VendorNVIDIA, []int{8, 16}},
	{"NVIDIA GeForce RTX 4060", machine.VendorNVIDIA, []int{8}},
	{"NVIDIA GeForce RTX 3090 Ti", machine.VendorNVIDIA, []int{24}},
	{"NVIDIA GeForce RTX 3090", machine.VendorNVIDIA, []int{24}},
	{"NVIDIA GeForce RTX 3080 Ti", machine.VendorNVIDIA, []int{12}},
	// The 12 GB 3080 is faster than discreteBandwidth's 10 GB figure.
	{"NVIDIA GeForce RTX 3080", machine.VendorNVIDIA, []int{10}},
	{"NVIDIA GeForce RTX 3070 Ti", machine.VendorNVIDIA, []int{8}},
	{"NVIDIA GeForce RTX 3070", machine.VendorNVIDIA, []int{8}},
	{"NVIDIA GeForce RTX 3060 Ti", machine.VendorNVIDIA, []int{8}},
	// As with the 3080, the bandwidth figure is the 12 GB card's.
	{"NVIDIA GeForce RTX 3060", machine.VendorNVIDIA, []int{12}},
	{"AMD Radeon RX 7900 XTX", machine.VendorAMD, []int{24}},
	{"AMD Radeon RX 7900 XT", machine.VendorAMD, []int{20}},
	{"AMD Radeon RX 7800 XT", machine.VendorAMD, []int{16}},
	{"AMD Radeon RX 9070 XT", machine.VendorAMD, []int{16}},
}

// findCard finds the one card query names, so "4090" is not the 4090 D.
func findCard(query string) (card, error) {
	q := spaced(query)
	var found []card
	for _, c := range cards {
		if q == spaced(c.name) || q == spaced(c.short()) {
			return c, nil
		}
		if isDevice(spaced(c.name), q) {
			found = append(found, c)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		names := make([]string, len(cards))
		for i, c := range cards {
			names[i] = c.short()
		}
		return card{}, fmt.Errorf("lo0 has no figures for a GPU called %q; the closest it knows are %s", query, orList(closest(query, names, 3)))
	default:
		names := make([]string, len(found))
		for i, c := range found {
			names[i] = c.short()
		}
		return card{}, fmt.Errorf("%q could be the %s; name one", query, orList(names))
	}
}

// coresSuffix matches a core count after a chip name, such as "32-core".
var coresSuffix = regexp.MustCompile(`(?i)[\s(]*(\d+)[\s-]*core(s)?(\s*gpu)?\)?\s*$`)

// simulateMac resolves an Apple chip, the full chip without a core count.
func simulateMac(query string, memory int) (Simulation, error) {
	name, cores := query, 0
	if m := coresSuffix.FindStringSubmatch(query); m != nil {
		name = query[:len(query)-len(m[0])]
		cores, _ = strconv.Atoi(m[1])
	}
	name = strings.TrimPrefix(spaced(name), "APPLE ")
	chip, ok := "", false
	var chips []string
	for _, s := range appleBandwidth {
		if !slices.Contains(chips, s.match) {
			chips = append(chips, s.match)
		}
		if spaced(s.match) == name {
			chip, ok = s.match, true
		}
	}
	if !ok {
		return Simulation{}, fmt.Errorf("lo0 has no figures for a Mac chip called %q; the closest it knows are %s", query, orList(closest(name, chips, 3)))
	}
	counts := chipCores(chip)
	switch {
	case cores == 0 && len(counts) > 0:
		cores = slices.Max(counts)
	case cores != 0 && !slices.Contains(counts, cores):
		return Simulation{}, fmt.Errorf("lo0 knows the %s with a %s-core GPU", chip, orList(counts))
	}
	if memory == 0 {
		return Simulation{}, fmt.Errorf("say how much unified memory the %s would have with --memory, such as --memory 64", chip)
	}
	return Simulation{
		gpu:    machine.GPU{Name: "Apple " + chip, Vendor: machine.VendorApple, Cores: cores, API: "Metal", Unified: true},
		count:  1,
		memory: uint64(memory) << 30,
	}, nil
}

// chipCores lists chip's known GPU core counts, fewest first.
func chipCores(chip string) []int {
	var out []int
	for _, r := range applePublished {
		if r.match == chip {
			out = append(out, r.gpuCores)
		}
	}
	for _, s := range appleBandwidth {
		if s.match == chip && s.gpuCores != 0 {
			out = append(out, s.gpuCores)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// spaced normalises a device name, so "rtx4090" equals "RTX 4090".
func spaced(s string) string {
	var b strings.Builder
	var prev byte
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c == '-' || c == '_' {
			c = ' '
		}
		letter, digit := c >= 'A' && c <= 'Z', c >= '0' && c <= '9'
		prevLetter, prevDigit := prev >= 'A' && prev <= 'Z', prev >= '0' && prev <= '9'
		if letter && prevDigit || digit && prevLetter {
			b.WriteByte(' ')
		}
		b.WriteByte(c)
		prev = c
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// closest returns up to n names nearest to query, nearest first.
func closest(query string, names []string, n int) []string {
	q := spaced(query)
	words := strings.Fields(q)
	shared := func(name string) int {
		count := 0
		for w := range strings.FieldsSeq(spaced(name)) {
			if slices.Contains(words, w) {
				count++
			}
		}
		return count
	}
	sorted := slices.Clone(names)
	slices.SortStableFunc(sorted, func(a, b string) int {
		return cmp.Or(cmp.Compare(shared(b), shared(a)), cmp.Compare(editDistance(q, spaced(a)), editDistance(q, spaced(b))))
	})
	return sorted[:min(n, len(sorted))]
}

// editDistance is the Levenshtein distance between a and b in bytes.
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// orList joins items as "a, b or c".
func orList[T any](items []T) string {
	s := make([]string, len(items))
	for i, v := range items {
		s[i] = fmt.Sprint(v)
	}
	if len(s) < 2 {
		return strings.Join(s, "")
	}
	return strings.Join(s[:len(s)-1], ", ") + " or " + s[len(s)-1]
}
