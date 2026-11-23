// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"slices"

	"github.com/mholt/archives"
)

// archiveDir writes the collected files under a single top-level directory.
func archiveDir(ctx context.Context, dir, destination string) error {
	files, err := archives.FilesFromDisk(ctx, nil, map[string]string{dir: ""})
	if err != nil {
		return err
	}
	files = slices.DeleteFunc(files, func(file archives.FileInfo) bool {
		return !file.IsDir() && !file.Mode().IsRegular()
	})

	// Archive names have second precision. Reserve the path without overwriting it.
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
	if err != nil {
		return err
	}
	if err := errors.Join(writeFlareZIP(ctx, out, files), out.Close()); err != nil {
		_ = os.Remove(destination)
		return err
	}
	return nil
}

func writeFlareZIP(ctx context.Context, out io.Writer, files []archives.FileInfo) error {
	// Archive discards errors when closing its ZIP writer. The buffer retains
	// write errors from that step and returns them from Flush.
	buffer := bufio.NewWriter(out)
	format := archives.Zip{Compression: zip.Deflate, SelectiveCompression: true}
	return errors.Join(format.Archive(ctx, buffer, files), buffer.Flush())
}
