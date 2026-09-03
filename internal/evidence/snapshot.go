package evidence

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	errSnapshotNotFound    = errors.New("snapshot entry not found")
	errSnapshotLimit       = errors.New("snapshot limit exceeded")
	errSnapshotOutside     = errors.New("snapshot path is outside repository")
	errSnapshotUnsupported = errors.New("snapshot entry is unsupported")
)

const maxSnapshotRecordBytes = 64 * 1024

// snapshot is the private seam between selected-snapshot analysis and its two
// real source adapters. Callers outside this package never coordinate it.
type snapshot interface {
	ReadDir(context.Context, string, int) ([]snapshotEntry, error)
	ReadFile(context.Context, string, int64) ([]byte, error)
}

type snapshotEntry struct {
	Name string
}

func selectedSnapshot(root, head string) snapshot {
	if head == WorkingTreeRevision {
		return &workingTreeSnapshot{root: root}
	}
	return &commitSnapshot{root: root, revision: head}
}

type commitSnapshot struct {
	root     string
	revision string
}

type commitTreeEntry struct {
	mode string
	kind string
	oid  string
	name string
}

func (snapshot *commitSnapshot) ReadDir(ctx context.Context, directory string, maximumEntries int) ([]snapshotEntry, error) {
	directory, err := cleanSnapshotDirectory(directory)
	if err != nil {
		return nil, err
	}
	treeish := snapshot.revision + "^{tree}"
	if directory != "" {
		entry, err := snapshot.entry(ctx, directory)
		if errors.Is(err, errSnapshotNotFound) {
			return []snapshotEntry{}, nil
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			return nil, errSnapshotUnsupported
		}
		if entry.kind != "tree" {
			return nil, errSnapshotUnsupported
		}
		treeish = entry.oid
	}
	records, exceeded, err := gitNULRecords(ctx, snapshot.root, maximumEntries, "ls-tree", "-z", treeish)
	if exceeded {
		return nil, errSnapshotLimit
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errSnapshotUnsupported
	}
	entries := make([]snapshotEntry, 0, len(records))
	for _, record := range records {
		entry, err := parseCommitTreeEntry(record)
		if err != nil {
			return nil, err
		}
		if entry.kind != "blob" {
			continue
		}
		relative, err := cleanSnapshotFile(entry.name)
		if err != nil {
			return nil, err
		}
		if path.Dir(relative) != "." {
			continue
		}
		name, err := cleanSnapshotName(path.Base(relative))
		if err != nil {
			return nil, err
		}
		entries = append(entries, snapshotEntry{Name: name})
	}
	return entries, nil
}

func (snapshot *commitSnapshot) ReadFile(ctx context.Context, relative string, maximumBytes int64) ([]byte, error) {
	relative, err := cleanSnapshotFile(relative)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	for depth := 0; depth < 32; depth++ {
		if _, duplicate := seen[relative]; duplicate {
			return nil, errSnapshotUnsupported
		}
		seen[relative] = struct{}{}
		entry, err := snapshot.entry(ctx, relative)
		if err != nil {
			return nil, err
		}
		if entry.kind != "blob" {
			return nil, errSnapshotUnsupported
		}
		if entry.mode == "120000" {
			target, err := snapshot.readBlob(ctx, entry.oid, maximumBytes)
			if err != nil {
				return nil, err
			}
			relative, err = resolveSnapshotLink(relative, string(target))
			if err != nil {
				return nil, err
			}
			continue
		}
		if !strings.HasPrefix(entry.mode, "100") {
			return nil, errSnapshotUnsupported
		}
		return snapshot.readBlob(ctx, entry.oid, maximumBytes)
	}
	return nil, errSnapshotUnsupported
}

func (snapshot *commitSnapshot) entry(ctx context.Context, relative string) (commitTreeEntry, error) {
	pathspec := ":(top,literal)" + relative
	records, exceeded, err := gitNULRecords(ctx, snapshot.root, 1, "ls-tree", "-z", "--full-tree", snapshot.revision, "--", pathspec)
	if err != nil {
		return commitTreeEntry{}, err
	}
	if exceeded || len(records) != 1 {
		return commitTreeEntry{}, errSnapshotNotFound
	}
	entry, err := parseCommitTreeEntry(records[0])
	if err != nil {
		return commitTreeEntry{}, err
	}
	if entry.name != relative {
		return commitTreeEntry{}, errSnapshotNotFound
	}
	return entry, nil
}

func (snapshot *commitSnapshot) readBlob(ctx context.Context, oid string, maximumBytes int64) ([]byte, error) {
	sizeOutput, exceeded, err := gitBytes(ctx, snapshot.root, 64, "cat-file", "-s", oid)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errSnapshotUnsupported
	}
	if exceeded {
		return nil, errSnapshotUnsupported
	}
	size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
	if err != nil || size < 0 {
		return nil, errSnapshotUnsupported
	}
	if size > maximumBytes {
		return nil, errSnapshotLimit
	}
	content, exceeded, err := gitBytes(ctx, snapshot.root, maximumBytes, "cat-file", "blob", oid)
	if exceeded {
		return nil, errSnapshotLimit
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errSnapshotUnsupported
	}
	if int64(len(content)) != size {
		return nil, errSnapshotUnsupported
	}
	return content, nil
}

func parseCommitTreeEntry(record string) (commitTreeEntry, error) {
	tab := strings.IndexByte(record, '\t')
	if tab < 0 {
		return commitTreeEntry{}, errSnapshotUnsupported
	}
	metadata := strings.Fields(record[:tab])
	if len(metadata) != 3 {
		return commitTreeEntry{}, errSnapshotUnsupported
	}
	return commitTreeEntry{
		mode: metadata[0],
		kind: metadata[1],
		oid:  metadata[2],
		name: record[tab+1:],
	}, nil
}

type workingTreeSnapshot struct {
	root string
}

func (snapshot *workingTreeSnapshot) ReadDir(ctx context.Context, directory string, maximumEntries int) ([]snapshotEntry, error) {
	directory, err := cleanSnapshotDirectory(directory)
	if err != nil {
		return nil, err
	}
	pattern := ":(top,glob)*"
	if directory != "" {
		pattern = ":(top,glob)" + gitGlobLiteral(directory) + "/*"
	}
	records, exceeded, err := gitNULRecords(
		ctx,
		snapshot.root,
		maximumEntries,
		"ls-files",
		"--cached",
		"--others",
		"--exclude-standard",
		"-z",
		"--",
		pattern,
	)
	if exceeded {
		return nil, errSnapshotLimit
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errSnapshotUnsupported
	}
	entries := make([]snapshotEntry, 0, len(records))
	for _, record := range records {
		relative, err := cleanSnapshotFile(record)
		if err != nil {
			return nil, err
		}
		if path.Dir(relative) != directory && !(directory == "" && path.Dir(relative) == ".") {
			continue
		}
		name, err := cleanSnapshotName(path.Base(relative))
		if err != nil {
			return nil, err
		}
		entries = append(entries, snapshotEntry{Name: name})
	}
	return entries, nil
}

func (snapshot *workingTreeSnapshot) ReadFile(ctx context.Context, relative string, maximumBytes int64) ([]byte, error) {
	if maximumBytes < 0 {
		return nil, errSnapshotLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relative, err := cleanSnapshotFile(relative)
	if err != nil {
		return nil, err
	}
	root, err := filepath.EvalSymlinks(snapshot.root)
	if err != nil {
		return nil, errSnapshotUnsupported
	}
	candidate := filepath.Join(root, filepath.FromSlash(relative))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errSnapshotNotFound
		}
		return nil, errSnapshotUnsupported
	}
	within, err := filepath.Rel(root, resolved)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return nil, errSnapshotOutside
	}
	file, err := os.Open(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errSnapshotNotFound
		}
		return nil, errSnapshotUnsupported
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !before.Mode().IsRegular() {
		return nil, errSnapshotUnsupported
	}
	if before.Size() > maximumBytes {
		return nil, errSnapshotLimit
	}
	content := make([]byte, 0)
	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		count, readErr := file.Read(buffer)
		if count > 0 {
			content = append(content, buffer[:count]...)
			if int64(len(content)) > maximumBytes {
				return nil, errSnapshotLimit
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, errSnapshotUnsupported
		}
	}
	after, err := file.Stat()
	if err != nil || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return nil, errSnapshotUnsupported
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return content, nil
}

func resolveSnapshotLink(relative, target string) (string, error) {
	if !utf8.ValidString(target) || target == "" || path.IsAbs(target) || hasControl(target) {
		return "", errSnapshotOutside
	}
	resolved := path.Clean(path.Join(path.Dir(relative), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", errSnapshotOutside
	}
	return cleanSnapshotFile(resolved)
}

func cleanSnapshotDirectory(directory string) (string, error) {
	if directory == "" || directory == "." {
		return "", nil
	}
	cleaned, err := cleanSnapshotFile(directory)
	if err != nil {
		return "", err
	}
	return cleaned, nil
}

func cleanSnapshotFile(relative string) (string, error) {
	if !utf8.ValidString(relative) || relative == "" || path.IsAbs(relative) || hasControl(relative) {
		return "", errSnapshotUnsupported
	}
	cleaned := path.Clean(relative)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || cleaned != relative {
		return "", errSnapshotOutside
	}
	return cleaned, nil
}

func cleanSnapshotName(name string) (string, error) {
	if !utf8.ValidString(name) || name == "" || name == "." || name == ".." || strings.Contains(name, "/") || hasControl(name) {
		return "", errSnapshotUnsupported
	}
	return name, nil
}

func gitGlobLiteral(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	for _, character := range value {
		if character == '\\' || character == '*' || character == '?' || character == '[' {
			result.WriteByte('\\')
		}
		result.WriteRune(character)
	}
	return result.String()
}

func hasControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func gitNULRecords(ctx context.Context, root string, maximumRecords int, args ...string) ([]string, bool, error) {
	ctx = withLocalOnlyGit(ctx)
	if maximumRecords < 0 {
		return nil, false, errSnapshotLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	commandContext, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(commandContext, "git", append([]string{"-C", root}, args...)...)
	command.Env = gitCommandEnvironment(commandContext)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, false, errSnapshotUnsupported
	}
	command.Stderr = &discardAfterLimit{maximum: 4 * 1024}
	if err := command.Start(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, false, contextErr
		}
		return nil, false, errSnapshotUnsupported
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Split(scanNUL)
	scanner.Buffer(make([]byte, 4096), maxSnapshotRecordBytes)
	records := make([]string, 0, min(maximumRecords, 32))
	exceeded := false
	for scanner.Scan() {
		if len(records) == maximumRecords {
			exceeded = true
			cancel()
			break
		}
		records = append(records, scanner.Text())
	}
	scanErr := scanner.Err()
	waitErr := command.Wait()
	if exceeded {
		return records, true, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if scanErr != nil {
		return nil, false, errSnapshotLimit
	}
	if waitErr != nil {
		return nil, false, errSnapshotUnsupported
	}
	return records, false, nil
}

func gitBytes(ctx context.Context, root string, maximumBytes int64, args ...string) ([]byte, bool, error) {
	ctx = withLocalOnlyGit(ctx)
	if maximumBytes < 0 {
		return nil, false, errSnapshotLimit
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	commandContext, cancel := context.WithCancel(ctx)
	defer cancel()
	command := exec.CommandContext(commandContext, "git", append([]string{"-C", root}, args...)...)
	command.Env = gitCommandEnvironment(commandContext)
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, false, errSnapshotUnsupported
	}
	command.Stderr = &discardAfterLimit{maximum: 4 * 1024}
	if err := command.Start(); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return nil, false, contextErr
		}
		return nil, false, errSnapshotUnsupported
	}
	content, readErr := io.ReadAll(io.LimitReader(stdout, maximumBytes+1))
	exceeded := int64(len(content)) > maximumBytes
	if exceeded {
		cancel()
	}
	waitErr := command.Wait()
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if readErr != nil {
		return nil, false, errSnapshotUnsupported
	}
	if exceeded {
		return content[:maximumBytes], true, nil
	}
	if waitErr != nil {
		return nil, false, errSnapshotUnsupported
	}
	return content, false, nil
}

func scanNUL(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if index := bytes.IndexByte(data, 0); index >= 0 {
		return index + 1, data[:index], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

type discardAfterLimit struct {
	maximum int
	written int
}

func (writer *discardAfterLimit) Write(content []byte) (int, error) {
	remaining := max(writer.maximum-writer.written, 0)
	writer.written += min(len(content), remaining)
	return len(content), nil
}
