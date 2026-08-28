package main

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assetDownloader struct {
	assets map[string][]byte
}

func (downloader assetDownloader) Download(_ context.Context, sourceURL, destination string, maxBytes int64) error {
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return err
	}
	assetName := filepath.Base(parsed.Path)
	data, ok := downloader.assets[assetName]
	if !ok {
		return fmt.Errorf("unexpected asset %s", assetName)
	}
	if int64(len(data)) > maxBytes {
		return fmt.Errorf("asset %s exceeds size limit", assetName)
	}
	return os.WriteFile(destination, data, 0o600)
}

type fakeCommandRunner struct {
	mu           sync.Mutex
	commands     []command
	tag          string
	composeImage string
}

func (runner *fakeCommandRunner) Run(_ context.Context, spec command) (string, error) {
	runner.mu.Lock()
	cloned := command{Name: spec.Name, Args: append([]string(nil), spec.Args...), Dir: spec.Dir, Stdout: spec.Stdout}
	runner.commands = append(runner.commands, cloned)
	runner.mu.Unlock()
	if spec.Stdout != nil {
		_, err := io.WriteString(spec.Stdout, "CREATE TABLE updater_test (id integer);\n")
		return "", err
	}
	joined := strings.Join(spec.Args, " ")
	switch {
	case strings.HasPrefix(joined, "compose --file ") && strings.HasSuffix(joined, " config --format json"):
		image := runner.composeImage
		if image == "" {
			image = "zeuszs:stable"
		}
		return fmt.Sprintf(`{"services":{"new-api":{"image":%q}}}`, image), nil
	case joined == "image inspect --format={{.Id}} zeuszs:stable":
		return "sha256:old-image\n", nil
	case strings.Contains(joined, "--entrypoint /usr/local/bin/new-api zeuszs:") && strings.HasSuffix(joined, " --version"):
		return runner.tag + "\n", nil
	case strings.HasPrefix(joined, "compose --file ") && strings.HasSuffix(joined, " ps -q new-api"):
		return "container-id\n", nil
	case joined == "inspect --format={{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}} container-id":
		return "healthy\n", nil
	default:
		return "", nil
	}
}

func (runner *fakeCommandRunner) snapshot() []command {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]command(nil), runner.commands...)
}

type sequencedHealthClient struct {
	mu       sync.Mutex
	versions []string
}

func (client *sequencedHealthClient) Version(context.Context, string) (string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.versions) == 0 {
		return "", fmt.Errorf("no health version configured")
	}
	value := client.versions[0]
	if len(client.versions) > 1 {
		client.versions = client.versions[1:]
	}
	return value, nil
}

func TestReleaseAssetNameUsesCurrentLinuxArchitecture(t *testing.T) {
	tests := []struct {
		goos   string
		goarch string
		want   string
		ok     bool
	}{
		{goos: "linux", goarch: "amd64", want: "new-api-zeuszs-v0.5.0", ok: true},
		{goos: "linux", goarch: "arm64", want: "new-api-arm64-zeuszs-v0.5.0", ok: true},
		{goos: "linux", goarch: "386"},
		{goos: "darwin", goarch: "amd64"},
	}
	for _, test := range tests {
		name, err := releaseAssetName(test.goos, test.goarch, "zeuszs-v0.5.0")
		if test.ok {
			require.NoError(t, err)
			assert.Equal(t, test.want, name)
		} else {
			assert.Error(t, err)
		}
	}
}

func TestVerifyFileSHA256RejectsAlteredRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new-api-zeuszs-v0.5.0")
	require.NoError(t, os.WriteFile(path, []byte("release"), 0o600))
	correct := sha256.Sum256([]byte("release"))
	require.NoError(t, verifyFileSHA256(path, correct[:]))

	altered := sha256.Sum256([]byte("altered"))
	assert.ErrorContains(t, verifyFileSHA256(path, altered[:]), "does not match")
}

func TestHTTPDownloaderRejectsOversizedAsset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, err := writer.Write([]byte("oversized"))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	destination := filepath.Join(t.TempDir(), "asset")

	err := (httpDownloader{client: server.Client()}).Download(context.Background(), server.URL, destination, 4)

	assert.ErrorContains(t, err, "size limit")
	_, statErr := os.Stat(destination)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestUpdaterConfigRequiresStrongNonPlaceholderToken(t *testing.T) {
	updater, _, _ := newUpdaterFixture(t, "zeuszs-v0.5.0", "zeuszs-v0.4.0", "zeuszs-v0.5.0")
	updater.cfg.Token = "short-token"
	assert.ErrorContains(t, updater.cfg.validate(), "at least 32")

	updater.cfg.Token = "replace-with-a-long-random-token"
	assert.ErrorContains(t, updater.cfg.validate(), "non-placeholder")

	updater.cfg.Token = "0123456789abcdef0123456789abcdef"
	require.NoError(t, updater.cfg.validate())
}

func TestDownloadReleaseVerifiesBinaryAndLicenseAssets(t *testing.T) {
	tag := "zeuszs-v0.5.0"
	assets := releaseAssets(tag)
	updater := &releaseUpdater{
		cfg:        config{ReleaseRoot: filepath.Join(t.TempDir(), "releases"), GOOS: "linux", GOARCH: "amd64"},
		downloader: assetDownloader{assets: assets},
	}

	releaseDir, binaryPath, err := updater.downloadRelease(context.Background(), tag)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(releaseDir, "new-api-"+tag), binaryPath)
	for _, name := range []string{"LICENSE", "NOTICE", "THIRD-PARTY-LICENSES.md"} {
		content, readErr := os.ReadFile(filepath.Join(releaseDir, name))
		require.NoError(t, readErr)
		assert.Equal(t, assets[name], content)
	}

	assets["new-api-"+tag] = []byte("tampered")
	_, _, err = updater.downloadRelease(context.Background(), tag)
	assert.ErrorContains(t, err, "does not match")
}

func TestReleaseUpdaterBacksUpAndOnlyRecreatesAppService(t *testing.T) {
	tag := "zeuszs-v0.5.0"
	updater, runner, root := newUpdaterFixture(t, tag, "zeuszs-v0.4.0", tag)

	err := updater.Execute(tag, func(string) {})
	require.NoError(t, err)

	commands := runner.snapshot()
	assert.True(t, hasCommand(commands, "exec new-api-postgres pg_dump --clean --if-exists --no-owner --no-privileges --username root --dbname new-api"))
	composeUp := strings.Join(updater.composeArgs("up", "-d", "--no-deps", "--force-recreate", "new-api"), " ")
	assert.True(t, hasCommand(commands, composeUp))
	assert.False(t, hasCommand(commands, "compose up -d"))
	assert.True(t, hasCommand(commands, "tag zeuszs:v0.5.0 zeuszs:stable"))

	backups, err := filepath.Glob(filepath.Join(root, "backups", "*", "postgres.sql.gz"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	file, err := os.Open(backups[0])
	require.NoError(t, err)
	defer file.Close()
	reader, err := gzip.NewReader(file)
	require.NoError(t, err)
	dump, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(dump), "CREATE TABLE updater_test")
	backupDir := filepath.Dir(backups[0])
	for _, name := range []string{"docker-compose.yml", "docker-compose.override.yml", ".env"} {
		info, statErr := os.Stat(filepath.Join(backupDir, name))
		require.NoError(t, statErr)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
	backupInfo, err := os.Stat(backupDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), backupInfo.Mode().Perm())

	dockerfile, err := os.ReadFile(filepath.Join(root, "releases", tag, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, string(dockerfile), "FROM "+defaultBaseImage)
	assert.Contains(t, string(dockerfile), "ca-certificates tzdata libasan8 wget")
	assert.Contains(t, string(dockerfile), "update-ca-certificates")
	assert.Contains(t, string(dockerfile), "WORKDIR /data")
	assert.Contains(t, string(dockerfile), "COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/")
}

func TestReleaseUpdaterRestoresStableImageWhenVersionCheckFails(t *testing.T) {
	tag := "zeuszs-v0.5.0"
	updater, runner, _ := newUpdaterFixture(t, tag, "zeuszs-v0.4.0", "zeuszs-v0.4.0")

	err := updater.Execute(tag, func(string) {})
	require.ErrorContains(t, err, "expected \"zeuszs-v0.5.0\"")

	commands := runner.snapshot()
	assert.True(t, hasCommand(commands, "tag zeuszs:v0.5.0 zeuszs:stable"))
	assert.True(t, hasCommand(commands, "tag sha256:old-image zeuszs:stable"))
	composeUp := strings.Join(updater.composeArgs("up", "-d", "--no-deps", "--force-recreate", "new-api"), " ")
	composePS := strings.Join(updater.composeArgs("ps", "-q", "new-api"), " ")
	assert.Equal(t, 2, countCommand(commands, composeUp))
	assert.Equal(t, 2, countCommand(commands, "inspect --format={{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}} container-id"))
	assert.Equal(t, 2, countCommand(commands, composePS))
}

func TestReleaseUpdaterRejectsReplayBeforeChangingDocker(t *testing.T) {
	tag := "zeuszs-v0.5.0"
	updater, runner, _ := newUpdaterFixture(t, tag, tag, tag)

	err := updater.Execute(tag, func(string) {})

	assert.ErrorContains(t, err, "must be newer")
	assert.Empty(t, runner.snapshot())
}

func TestReleaseUpdaterRejectsUnexpectedComposeImage(t *testing.T) {
	tag := "zeuszs-v0.5.0"
	updater, runner, _ := newUpdaterFixture(t, tag, "zeuszs-v0.4.0", tag)
	runner.composeImage = "untrusted/app:latest"

	err := updater.Execute(tag, func(string) {})

	assert.ErrorContains(t, err, "must use exactly zeuszs:stable")
	assert.False(t, hasCommand(runner.snapshot(), "image inspect --format={{.Id}} zeuszs:stable"))
}

func TestCreateBackupRejectsSymlinkedDeploymentFiles(t *testing.T) {
	root := t.TempDir()
	composeDir := filepath.Join(root, "new-api")
	require.NoError(t, os.MkdirAll(composeDir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(composeDir, "docker-compose.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(composeDir, "docker-compose.override.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "outside.env"), []byte("SECRET=value\n"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(root, "outside.env"), filepath.Join(composeDir, ".env")))
	updater := &releaseUpdater{
		cfg: config{
			BackupRoot:          filepath.Join(root, "backups"),
			ComposeDir:          composeDir,
			ComposeFile:         filepath.Join(composeDir, "docker-compose.yml"),
			ComposeOverrideFile: filepath.Join(composeDir, "docker-compose.override.yml"),
		},
		now: func() time.Time { return time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC) },
	}

	_, err := updater.createBackup("zeuszs-v0.5.0")
	assert.ErrorContains(t, err, "non-symlink")
}

func newUpdaterFixture(t *testing.T, tag, currentVersion, healthVersion string) (*releaseUpdater, *fakeCommandRunner, string) {
	t.Helper()
	root := t.TempDir()
	composeDir := filepath.Join(root, "new-api")
	require.NoError(t, os.MkdirAll(composeDir, 0o700))
	composeFile := filepath.Join(composeDir, "docker-compose.yml")
	require.NoError(t, os.WriteFile(composeFile, []byte("services:\n  new-api:\n    image: zeuszs:stable\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(composeDir, "docker-compose.override.yml"), []byte("services:\n  new-api:\n    restart: always\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(composeDir, ".env"), []byte("COMPOSE_PROJECT_NAME=zeuszs\n"), 0o600))
	runner := &fakeCommandRunner{tag: tag}
	updater := &releaseUpdater{
		cfg: config{
			SocketPath:          filepath.Join(root, "updater.sock"),
			ReleaseRoot:         filepath.Join(root, "releases"),
			BackupRoot:          filepath.Join(root, "backups"),
			StateRoot:           filepath.Join(root, "state"),
			ComposeDir:          composeDir,
			ComposeFile:         composeFile,
			ComposeOverrideFile: filepath.Join(composeDir, "docker-compose.override.yml"),
			DockerBinary:        "/usr/bin/docker",
			DockerRoot:          root,
			AppService:          "new-api",
			PostgresContainer:   "new-api-postgres",
			PostgresUser:        "root",
			PostgresDatabase:    "new-api",
			HealthURL:           "http://127.0.0.1:3000/api/status",
			BaseImage:           defaultBaseImage,
			GOOS:                "linux",
			GOARCH:              "amd64",
			HealthTimeout:       time.Second,
			PollInterval:        time.Millisecond,
			UpdateTimeout:       time.Minute,
			RollbackTimeout:     time.Minute,
			MinFreeBytes:        1,
		},
		runner:     runner,
		downloader: assetDownloader{assets: releaseAssets(tag)},
		health:     &sequencedHealthClient{versions: []string{currentVersion, healthVersion}},
		now:        func() time.Time { return time.Date(2026, 8, 28, 10, 0, 0, 0, time.UTC) },
	}
	return updater, runner, root
}

func releaseAssets(tag string) map[string][]byte {
	assets := map[string][]byte{
		"new-api-" + tag:          []byte("binary"),
		"LICENSE":                 []byte("license"),
		"NOTICE":                  []byte("notice"),
		"THIRD-PARTY-LICENSES.md": []byte("third party licenses"),
	}
	var checksums strings.Builder
	for _, name := range []string{"new-api-" + tag, "LICENSE", "NOTICE", "THIRD-PARTY-LICENSES.md"} {
		digest := sha256.Sum256(assets[name])
		fmt.Fprintf(&checksums, "%x  %s\n", digest, name)
	}
	assets["checksums-linux.txt"] = []byte(checksums.String())
	return assets
}

func hasCommand(commands []command, args string) bool {
	return countCommand(commands, args) > 0
}

func countCommand(commands []command, args string) int {
	count := 0
	for _, command := range commands {
		if strings.Join(command.Args, " ") == args {
			count++
		}
	}
	return count
}
