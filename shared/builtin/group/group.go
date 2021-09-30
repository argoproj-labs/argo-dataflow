package group

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"

	"github.com/antonmedv/expr"
	dfv1 "github.com/argoproj-labs/argo-dataflow/api/v1alpha1"
	"github.com/argoproj-labs/argo-dataflow/runner/util"
	"github.com/argoproj-labs/argo-dataflow/shared/builtin"
	sharedutil "github.com/argoproj-labs/argo-dataflow/shared/util"
	"github.com/google/uuid"
	"github.com/juju/fslock"
)

func withLock(dir string, f func() ([]byte, error)) ([]byte, error) {
	mu := fslock.New(fmt.Sprintf("%s.lock", dir))
	if err := mu.Lock(); err != nil {
		return nil, fmt.Errorf("failed to lock %s %w", dir, err)
	}
	defer func() { _ = mu.Unlock() }()
	msgs, err := f()
	if err := mu.Unlock(); err != nil {
		return nil, fmt.Errorf("failed to unlock %s %w", dir, err)
	}
	return msgs, err
}

func New(pathGroups, key, endOfGroup string, groupFormat dfv1.GroupFormat) (builtin.Process, error) {
	if err := os.Mkdir(pathGroups, 0o700); sharedutil.IgnoreExist(err) != nil {
		return nil, fmt.Errorf("failed to create groups dir: %w", err)
	}
	prog, err := expr.Compile(key)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %q: %w", key, err)
	}
	endProg, err := expr.Compile(endOfGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to compile %q: %w", endOfGroup, err)
	}
	return func(ctx context.Context, msg []byte) ([]byte, error) {
		env, err := util.ExprEnv(ctx, msg)
		if err != nil {
			return nil, fmt.Errorf("failed to create expr env: %w", err)
		}
		res, err := expr.Run(prog, env)
		if err != nil {
			return nil, fmt.Errorf("failed to run program: %w", err)
		}
		group, ok := res.(string)
		if !ok {
			return nil, fmt.Errorf("key expression must return a string")
		}
		dir := filepath.Join(pathGroups, group)
		return withLock(dir, func() ([]byte, error) {
			if err := os.MkdirAll(dir, 0o700); sharedutil.IgnoreExist(err) != nil {
				return nil, fmt.Errorf("failed to create group sub-dir %q: %w", dir, err)
			}
			path := filepath.Join(dir, uuid.New().String())
			if err := ioutil.WriteFile(path, msg, 0o600); err != nil {
				return nil, fmt.Errorf("failed to create message file %q: %w", path, err)
			}
			res, err = expr.Run(endProg, env)
			if err != nil {
				return nil, fmt.Errorf("failed to run program: %w", err)
			}
			end, ok := res.(bool)
			if !ok {
				return nil, fmt.Errorf("end-of-group expression must return a bool")
			}
			if !end {
				return nil, nil
			}
			items, err := ioutil.ReadDir(dir)
			if err != nil {
				return nil, fmt.Errorf("failed to read dir: %w", err)
			}
			// return items is creating date order, this is only at accuracy of system clock
			sort.Slice(items, func(i, j int) bool {
				return items[i].ModTime().Before(items[j].ModTime())
			})
			msgs := make([][]byte, len(items))
			for i, f := range items {
				msg, err := ioutil.ReadFile(filepath.Join(pathGroups, group, f.Name()))
				if err != nil {
					return nil, fmt.Errorf("failed to read file %q: %w", f.Name(), err)
				}
				msgs[i] = msg
			}
			switch groupFormat {
			case dfv1.GroupFormatUnknown:
			// noop - this is same as default switch branch
			case dfv1.GroupFormatJSONBytesArray:
				data, err := json.Marshal(msgs)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal messages: %w", err)
				}
				return data, os.RemoveAll(dir)
			case dfv1.GroupFormatJSONStringArray:
				stringMsgs := make([]string, len(items))
				for i, bytes := range msgs {
					stringMsgs[i] = string(bytes)
				}
				data, err := json.Marshal(stringMsgs)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal messages: %w", err)
				}
				return data, os.RemoveAll(dir)
			}
			return nil, fmt.Errorf("unknown group format %q", groupFormat)
		})
	}, nil
}
