package utils

import (
	"crypto/md5" //nolint:gosec
	"encoding/hex"
	"errors"

	uid "github.com/gofrs/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// label on a node indicates the current maintenance in-progress
	MAINTENANCE_LABEL_PREFIX = "maintenance/"
	MAINTENANCE_CLIENT_LABEL = "maintenance-client"

	MAINTENANCE_METADATA_OVERRIDES_KEY = MAINTENANCE_LABEL_PREFIX + "maintenanceMetadataOverrides"
	DefaultMaintenanceClient           = "Slack_ST2"

	CORDONED_LABEL_KEY = "cordoned"
)

var (
	NM_STATE_OLD = errors.New("requeue: NotifyMaintenance state is old")
)

func IsFinal(object metav1.Object) bool {
	return !object.GetDeletionTimestamp().IsZero()
}

func Md5Hash(obj string) string {
	hash := md5.Sum([]byte(obj)) //nolint:gosec
	return hex.EncodeToString(hash[:])
}

func DeterministicUID(in string) string {
	// Calculate the MD5 hash of the input string
	md5string := Md5Hash(in)
	uuid, err := uid.FromString(md5string)
	if err != nil {
		return ""
	}
	return uuid.String()
}
