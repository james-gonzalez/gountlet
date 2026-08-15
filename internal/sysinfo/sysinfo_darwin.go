//go:build darwin

package sysinfo

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func sysctl(name string) string {
	out, err := exec.Command("sysctl", "-n", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func fillCPU(c *CPU) {
	c.Model = sysctl("machdep.cpu.brand_string")
	if v, err := strconv.Atoi(sysctl("hw.physicalcpu")); err == nil {
		c.PhysicalCores = v
	} else {
		c.PhysicalCores = c.LogicalCores
	}
}

func fillMemory(m *Memory) {
	if v, err := strconv.ParseUint(sysctl("hw.memsize"), 10, 64); err == nil {
		m.TotalBytes = v
	}

	out, err := exec.Command("system_profiler", "SPMemoryDataType").Output()
	if err != nil {
		return
	}
	typeRe := regexp.MustCompile(`(?m)^\s*Type:\s*(DDR\w*)\s*$`)
	speedRe := regexp.MustCompile(`(?m)^\s*Speed:\s*(\d+)\s*MHz\s*$`)
	if mm := typeRe.FindStringSubmatch(string(out)); mm != nil {
		m.Type = mm[1]
	}
	if mm := speedRe.FindStringSubmatch(string(out)); mm != nil {
		if v, err := strconv.Atoi(mm[1]); err == nil {
			m.SpeedMHz = v
		}
	}
}

func fillDisk(d *Disk, path string) {
	out, err := exec.Command("df", "-k", path).Output()
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 4 {
		return
	}
	d.Device = fields[0]
	if kb, err := strconv.ParseUint(fields[1], 10, 64); err == nil {
		d.TotalBytes = kb * 1024
	}
	if kb, err := strconv.ParseUint(fields[3], 10, 64); err == nil {
		d.FreeBytes = kb * 1024
	}

	if out, err := exec.Command("diskutil", "info", d.Device).Output(); err == nil {
		fsRe := regexp.MustCompile(`(?m)^\s*Type \(Bundle\):\s*(\S+)\s*$`)
		nameRe := regexp.MustCompile(`(?m)^\s*(?:Device / Media Name|Media Name):\s*(.+?)\s*$`)
		if mm := fsRe.FindStringSubmatch(string(out)); mm != nil {
			d.Filesystem = mm[1]
		}
		if mm := nameRe.FindStringSubmatch(string(out)); mm != nil {
			d.Model = mm[1]
		}
	}
}

func fillNetLinkSpeed(n *NetInterface) {
	if n.Name == "" {
		return
	}
	out, err := exec.Command("ifconfig", n.Name).Output()
	if err != nil {
		return
	}
	// Looks for a media line like "media: autoselect (1000baseT <full-duplex>)".
	re := regexp.MustCompile(`(\d+)base`)
	if mm := re.FindStringSubmatch(string(out)); mm != nil {
		if v, err := strconv.Atoi(mm[1]); err == nil {
			n.LinkMbps = v
		}
	}
}
