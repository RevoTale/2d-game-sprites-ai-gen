package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func Read(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	values := map[string]string{}
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("%s:%d: expected KEY=value", path, lineNumber)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("%s:%d: key is required", path, lineNumber)
		}
		values[key] = unquote(strings.TrimSpace(value))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func Merge(process []string, files ...string) (map[string]string, error) {
	values := map[string]string{}
	for _, file := range files {
		fileValues, err := Read(file)
		if err != nil {
			return nil, err
		}
		for key, value := range fileValues {
			values[key] = value
		}
	}
	for _, item := range process {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		values[key] = value
	}
	return values, nil
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	first := value[0]
	last := value[len(value)-1]
	if first == last && (first == '\'' || first == '"') {
		return value[1 : len(value)-1]
	}
	return value
}
