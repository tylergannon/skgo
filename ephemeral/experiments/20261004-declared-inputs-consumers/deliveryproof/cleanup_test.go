package deliveryproof

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestDeliveryOwnedC512ReceiptCleanup(t *testing.T) {
	logs := []string{"product-final-c512e83.log", "delivery/consumer-c512e83.log", "delivery/consumer-controls-c512e83.log", "delivery/native-worker-guard-c512e83.log", "delivery/individual-refusals-c512e83.log"}
	pattern := regexp.MustCompile(`command pid/group=(\d+)`)
	groups := map[int]bool{}
	for _, log := range logs {
		for _, match := range pattern.FindAllStringSubmatch(content(t, filepath.Join(proofRoot, log)), -1) {
			pid, err := strconv.Atoi(match[1])
			if err != nil {
				t.Fatal(err)
			}
			groups[pid] = true
		}
	}
	var receipts []string
	for _, glob := range []string{"product-fixtures/*sdk-c512e83/body.ready", "product-fixtures/*sdk-c512e83/owned-pids.log", "delivery-fixtures/guard-native-worker-sdk-c512e83*/body.ready"} {
		matches, err := filepath.Glob(filepath.Join(proofRoot, glob))
		if err != nil {
			t.Fatal(err)
		}
		receipts = append(receipts, matches...)
	}
	if len(receipts) != 7 {
		t.Fatalf("owned lifecycle receipts %d; want literal7", len(receipts))
	}
	dirs := map[string]bool{}
	for _, path := range receipts {
		for _, line := range strings.Split(content(t, path), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err != nil {
				continue
			}
			if !strings.HasPrefix(fields[1], filepath.Join(os.TempDir(), "skgo-prerender-")) {
				t.Fatalf("unexpected owned temporary receipt %q", line)
			}
			groups[pid] = true
			dirs[fields[1]] = true
		}
	}
	for pid := range groups {
		if err := syscall.Kill(-pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("owned group%d remains or is inaccessible:%v", pid, err)
		}
	}
	for dir := range dirs {
		if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("owned private directory remains or is inaccessible:%s:%v", dir, err)
		}
	}
	t.Logf("direct owned checks: command/body groups%d absent; private temporary directories%d absent; receipts%d", len(groups), len(dirs), len(receipts))
}
