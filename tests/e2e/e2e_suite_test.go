package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	probeURL   string
	httpClient *http.Client
)

const (
	defaultProbeURL     = "http://localhost:8081"
	startupTimeout      = 30 * time.Second
	startupPollInterval = 500 * time.Millisecond
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Worker E2E Test Suite")
}

var _ = BeforeSuite(func() {
	probeURL = os.Getenv("PROBE_URL")
	if probeURL == "" {
		probeURL = defaultProbeURL
	}
	probeURL = strings.TrimRight(probeURL, "/")

	httpClient = &http.Client{
		Timeout: 5 * time.Second,
	}

	Eventually(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probeURL+"/livez", nil)
		if err != nil {
			return err
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d from /livez", resp.StatusCode)
		}
		return nil
	}).WithTimeout(startupTimeout).WithPolling(startupPollInterval).Should(
		Succeed(),
		"Target worker probe %s did not become healthy within %s",
		probeURL,
		startupTimeout,
	)
})

var _ = AfterSuite(func() {
	// Teardown if any
})
