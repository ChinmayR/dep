package xhttp

import (
	"encoding/json"
	"net/http"
)

// RespondWithJSON sends a json response
func RespondWithJSON(w http.ResponseWriter, data interface{}) error {
	w.Header().Add("Content-Type", MIMETypeApplicationJSON)
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}

	_, err = w.Write(encoded)
	return err
}
