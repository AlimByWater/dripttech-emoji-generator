package models

import (
	"encoding/json"
	"io"
)

type InputFileType int

// InputFile https://core.telegram.org/bots/api#inputfile
type InputFile interface {
	inputFileTag()
}

type InputFileUpload struct {
	Filename string
	Data     io.Reader
}

func (*InputFileUpload) inputFileTag() {}

func (i *InputFileUpload) MarshalJSON() ([]byte, error) {
	return []byte(`"@` + i.Filename + `"`), nil
}

type InputFileBytes struct {
	Filename string
	Reader   io.Reader
}

func (*InputFileBytes) inputFileTag() {}

func (i *InputFileBytes) MarshalJSON() ([]byte, error) {
	var byt []byte
	for {
		buf := make([]byte, 1024)
		n, err := i.Reader.Read(buf)
		if err != nil && err != io.EOF {
			return nil, err
		}
		if n == 0 {
			break
		}
		byt = append(byt, buf[:n]...)
	}
	return byt, nil
}

type InputFileString struct {
	Data string
}

func (*InputFileString) inputFileTag() {}

func (i *InputFileString) MarshalJSON() ([]byte, error) {
	return []byte(`"` + i.Data + `"`), nil
}

func (i *InputFileString) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &i.Data)
}
