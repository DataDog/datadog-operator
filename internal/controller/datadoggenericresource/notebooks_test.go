package datadoggenericcr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_notebookStringToInt64(t *testing.T) {
	notebookStringID := "123"
	expectedNotebookID := int64(123)
	notebookID, err := notebookStringToInt64(notebookStringID)
	assert.NoError(t, err)
	assert.Equal(t, expectedNotebookID, notebookID)

	// Invalid notebook ID
	notebookStringID = "invalid"
	notebookID, err = notebookStringToInt64(notebookStringID)
	assert.EqualError(t, err, "error parsing notebook Id: strconv.ParseInt: parsing \"invalid\": invalid syntax")
}

func Test_notebookInt64ToString(t *testing.T) {
	notebookID := int64(123)
	expectedNotebookStringID := "123"
	notebookStringID := notebookInt64ToString(notebookID)
	assert.Equal(t, expectedNotebookStringID, notebookStringID)
}
