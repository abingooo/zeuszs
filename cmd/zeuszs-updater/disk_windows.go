package main

import "errors"

func availableDiskBytes(string) (uint64, error) {
	return 0, errors.New("zeuszs-updater is only supported on Linux hosts")
}
