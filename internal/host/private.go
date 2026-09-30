package host

import "os"

// Only newly created paths are secured automatically. Existing unsafe paths
// are refused rather than silently changing permissions on user data.
func MkdirPrivate(path string) error {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		if err = mkdirPrivate(path); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return CheckPrivate(path, true)
}
