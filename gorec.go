package gorec

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	GoogleEndpoint = "https://www.google.com/speech-api/v2/recognize?lang=%s&output=json&key=%s"
	ContentType    = "audio/l16; rate=16000;"
)
const (
	English Language = iota
	Spanish
	French
	Greek
	German
	Italian
)

var langs = [][]string{
	[]string{"en-gb", "English"},
	[]string{"es-es", "Spanish"},
	[]string{"fr-fr", "French"},
	[]string{"el", "Greek"},
	[]string{"de-de", "German"},
	[]string{"it-it", "Italian"},
}

var SupportedLanguages = []Language{
	English,
	Spanish,
	French,
	Greek,
	German,
	Italian,
}

type Language int

func (l Language) StringCode() string           { return langs[l][0] }
func (l Language) String() string               { return langs[l][1] }
func (l Language) MarshalJSON() ([]byte, error) { return []byte(l.String()), nil }

type Alternative struct {
	Transcript string  `json:"transcript"`
	Confidence float64 `json:"confidence"`
}
type Result struct {
	Alternatives []Alternative `json:"alternative"`
	Final        bool          `json:"final"`
}

type GoogleResponse struct {
	Results     []Result `json:"result"`
	ResultIndex int      `json:"result_index"`
}

type Hypothesis struct {
	Alternative Alternative `json:"text"`
	Language    Language    `json:"language"`
	Err         error       `json:-`
}

func (h Hypothesis) String() string {
	bytes, err := json.Marshal(h)
	if err != nil {
		return ""
	}
	return string(bytes)
}

func ListenFile(audio []byte, key string) (*Hypothesis, error) {
	var best *Hypothesis
	c := make(chan Hypothesis)
	if err != nil {
		return nil, err
	}
	for _, lang := range SupportedLanguages {
		go checkLanguage(audio, key, lang, c)
	}
	for remaining := len(SupportedLanguages); remaining > 0; remaining-- {
		select {
		case h := <-c:
			if h.Err == nil {
				if best == nil || best.Alternative.Confidence < h.Alternative.Confidence {
					best = &h
				}
			}
		case <-time.After(30 * time.Second):
			break
		}
	}
	if best == nil {
		return nil, errors.New("No response")
	}
	return best, nil
}

func checkLanguage(audio []byte, key string, lang Language, c chan Hypothesis) {
	h := Hypothesis{Language: lang}
	str, err := sendFile(audio, key, lang)
	if err != nil {
		h.Err = err
		c <- h
		return
	}
	gr := &GoogleResponse{}
	err = json.Unmarshal([]byte(str), gr)
	if err != nil {
		h.Err = err
		c <- h
		return
	}
	alt := checkAlternatives(gr)
	h.Alternative = *alt
	c <- h
}

func sendFile(audio []byte, key string, lang Language) (string, error) {
	r, err := http.NewRequest("POST", fmt.Sprintf(GoogleEndpoint, lang.StringCode(), key), bytes.NewBuffer(audio))
	if err != nil {
		return "", err
	}
	r.Header.Set("Content-Type", ContentType)

	client := &http.Client{}
	resp, err := client.Do(r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyByte, _ := ioutil.ReadAll(resp.Body)
	body := strings.TrimPrefix(string(bodyByte), "{\"result\":[]}\n")
	return string(body), nil
}

func ReadAudioFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	var size int64 = info.Size()
	bytes := make([]byte, size)

	buffer := bufio.NewReader(file)
	_, err = buffer.Read(bytes)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func checkAlternatives(gr *GoogleResponse) *Alternative {
	if len(gr.Results) == 0 || len(gr.Results[0].Alternatives) == 0 {
		return nil
	}
	return &gr.Results[0].Alternatives[0]
}
