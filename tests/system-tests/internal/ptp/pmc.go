package ptp

import (
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// discoverPtp4lConfigPaths lists /var/run/ptp4l.<n>.config files in the linuxptp-daemon pod.
// Falls back to /var/run/ptp4l.0.config if glob matches nothing (older layouts).
func discoverPtp4lConfigPaths(apiClient *clients.Settings, nodeName string) ([]string, error) {
	daemonPod, err := GetLinuxptpDaemonPodOnNode(apiClient, nodeName)
	if err != nil {
		return nil, err
	}

	buf, err := daemonPod.ExecCommand(
		[]string{"sh", "-c", "ls -1 /var/run/ptp4l.*.config 2>/dev/null || true"},
		DaemonContainerName,
	)
	if err != nil {
		return nil, fmt.Errorf("list ptp4l configs: %w", err)
	}

	var paths []string

	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}

	if len(paths) == 0 {
		paths = []string{"/var/run/ptp4l.0.config"}
	}

	return paths, nil
}

// shellQuoteSingle wraps s in single quotes for POSIX sh -c.
func shellQuoteSingle(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'"'"'`) + `'`
}

// PmcGetParentDataSet runs `pmc -u -b 0 -f <config> "GET PARENT_DATA_SET"` for each discovered
// ptp4l config until output contains gm.ClockClass (grandmaster data). linuxptp pmc needs -f to
// locate the UDS; without it only "sending: ..." appears and no RESPONSE is returned.
func PmcGetParentDataSet(apiClient *clients.Settings, nodeName string) (string, error) {
	paths, err := discoverPtp4lConfigPaths(apiClient, nodeName)
	if err != nil {
		return "", err
	}

	daemonPod, err := GetLinuxptpDaemonPodOnNode(apiClient, nodeName)
	if err != nil {
		return "", err
	}

	var lastOut string

	for _, cfg := range paths {
		cmd := fmt.Sprintf("pmc -u -b 0 -f %s \"GET PARENT_DATA_SET\" 2>&1", shellQuoteSingle(cfg))
		buf, execErr := daemonPod.ExecCommand(
			[]string{"sh", "-c", cmd},
			DaemonContainerName,
		)
		lastOut = buf.String()
		if execErr != nil {
			continue
		}

		if strings.Contains(lastOut, "gm.ClockClass") {
			return lastOut, nil
		}
	}

	return lastOut, fmt.Errorf(
		"pmc GET PARENT_DATA_SET: no profile returned gm.ClockClass; tried %v; last output: %s",
		paths, strings.TrimSpace(lastOut))
}

// PmcGetTimeStatusNP runs `pmc -u -b 0 -f <config> "GET TIME_STATUS_NP"` using the first config
// that produces a non-empty response after the "sending:" line (socket resolved).
func PmcGetTimeStatusNP(apiClient *clients.Settings, nodeName string) (string, error) {
	paths, err := discoverPtp4lConfigPaths(apiClient, nodeName)
	if err != nil {
		return "", err
	}

	daemonPod, err := GetLinuxptpDaemonPodOnNode(apiClient, nodeName)
	if err != nil {
		return "", err
	}

	var lastOut string

	for _, cfg := range paths {
		cmd := fmt.Sprintf("pmc -u -b 0 -f %s \"GET TIME_STATUS_NP\" 2>&1", shellQuoteSingle(cfg))
		buf, execErr := daemonPod.ExecCommand(
			[]string{"sh", "-c", cmd},
			DaemonContainerName,
		)
		lastOut = buf.String()
		if execErr != nil {
			continue
		}

		if strings.Contains(lastOut, "RESPONSE") || strings.Contains(lastOut, "timeTraceable") {
			return lastOut, nil
		}
	}

	return lastOut, fmt.Errorf(
		"pmc GET TIME_STATUS_NP: no usable response; tried %v; last output: %s",
		paths, strings.TrimSpace(lastOut))
}
