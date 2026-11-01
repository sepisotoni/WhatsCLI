package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const releaseURL = "https://api.github.com/repos/sepisotoni/WhatsCLI/releases/latest"

var BuildVersion = "dev"

type Version struct {
	Current   string
	Latest    string
	Available bool
}

type semVersion struct {
	core       [3]uint64
	prerelease bool
}

func Check(ctx context.Context, current string) (Version, error) {
	return check(ctx, current, releaseURL, &http.Client{Timeout: 10 * time.Second})
}

func check(ctx context.Context, current, endpoint string, client *http.Client) (Version, error) {
	currentParts, err := parseVersion(current)
	if err != nil {
		return Version{}, errors.New("this build has no valid embedded release version")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Version{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "WhatsCLI-update-check")
	resp, err := client.Do(req)
	if err != nil {
		return Version{}, errors.New("could not reach GitHub to check for updates")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Version{}, fmt.Errorf("GitHub update check returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return Version{}, errors.New("GitHub returned an invalid update response")
	}
	latestParts, err := parseVersion(result.Tag)
	if err != nil || latestParts.prerelease {
		return Version{}, errors.New("GitHub returned an invalid release version")
	}
	return Version{
		Current:   strings.TrimPrefix(current, "v"),
		Latest:    strings.TrimPrefix(result.Tag, "v"),
		Available: compareVersion(latestParts, currentParts) > 0,
	}, nil
}

func parseVersion(version string) (semVersion, error) {
	var parsed semVersion
	version = strings.TrimPrefix(version, "v")
	coreAndPrerelease := strings.SplitN(version, "-", 2)
	if len(coreAndPrerelease) == 2 {
		if !validPrerelease(coreAndPrerelease[1]) {
			return parsed, errors.New("invalid semantic version")
		}
		parsed.prerelease = true
	}
	fields := strings.Split(coreAndPrerelease[0], ".")
	parts := &parsed.core
	if len(fields) != len(parts) {
		return parsed, errors.New("invalid semantic version")
	}
	for i, field := range fields {
		if field == "" {
			return parsed, errors.New("invalid semantic version")
		}
		if len(field) > 1 && field[0] == '0' {
			return parsed, errors.New("invalid semantic version")
		}
		for _, char := range field {
			if char < '0' || char > '9' {
				return parsed, errors.New("invalid semantic version")
			}
		}
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return parsed, errors.New("invalid semantic version")
		}
		parts[i] = value
	}
	return parsed, nil
}

func validPrerelease(value string) bool {
	if value == "" {
		return false
	}
	for _, identifier := range strings.Split(value, ".") {
		if identifier == "" {
			return false
		}
		for _, char := range identifier {
			if !(char >= '0' && char <= '9' || char >= 'A' && char <= 'Z' ||
				char >= 'a' && char <= 'z' || char == '-') {
				return false
			}
		}
	}
	return true
}

func compareVersion(left, right semVersion) int {
	for i := range left.core {
		if left.core[i] > right.core[i] {
			return 1
		}
		if left.core[i] < right.core[i] {
			return -1
		}
	}
	if left.prerelease != right.prerelease {
		if left.prerelease {
			return -1
		}
		return 1
	}
	return 0
}

func ScriptPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", errors.New("could not locate the running WhatsCLI executable")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", errors.New("could not resolve the running WhatsCLI executable")
	}
	path := filepath.Join(filepath.Dir(filepath.Dir(executable)), "install.sh")
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("the install script is unavailable; reinstall WhatsCLI before updating")
	}
	return path, nil
}

func Run(ctx context.Context) (string, error) {
	script, err := ScriptPath()
	if err != nil {
		return "", err
	}
	args := []string{"--update"}
	if strings.Contains(BuildVersion, "-beta.") {
		args = append(args, "--beta")
	}
	commandArgs := append([]string{script}, args...)
	cmd := exec.CommandContext(ctx, "sh", commandArgs...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "WHATSCLI_DIR=") && !strings.HasPrefix(entry, "WHATSCLI_REF=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, "WHATSCLI_DIR="+filepath.Dir(script))
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if text == "" {
			return "", errors.New("update failed; see the installation directory and retry from a terminal")
		}
		return text, fmt.Errorf("update failed: %s", text)
	}
	return text, nil
}
