package sources

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"
)

// dockerRun runs a Docker container with automatic cleanup on timeout.
// When a context deadline is exceeded, Go kills the docker CLI client but the
// container keeps running in the daemon. This function names each container and
// force-removes it after completion to prevent orphaned containers.
func dockerRun(ctx context.Context, timeout time.Duration, prefix string, args ...string) ([]byte, []byte, error) {
	containerName := fmt.Sprintf("qsos-%s-%d", prefix, time.Now().UnixNano())

	dockerCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullArgs := make([]string, 0, len(args)+4)
	fullArgs = append(fullArgs, "run", "--rm", "--name", containerName)
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(dockerCtx, "docker", fullArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Always force-remove the container to handle timeout cases.
	rmCtx, rmCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer rmCancel()
	if rmErr := exec.CommandContext(rmCtx, "docker", "rm", "-f", containerName).Run(); rmErr == nil && err != nil {
		log.Printf("  Cleaned up orphaned container %s", containerName)
	}

	return stdout.Bytes(), stderr.Bytes(), err
}
