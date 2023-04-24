package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"os"
	"sync"
)

type Config struct {
	Keys map[string][]string `json:"keys"`
}

var config Config
var keyIndex sync.Map

func loadConfig() {
	configPath := "config/config.json"
	file, err := os.Open(configPath)
	if err != nil {
		cwd, _ := os.Getwd()
		fullPath := filepath.Join(cwd, configPath)
		log.Fatalf("Error opening config file at path %s: %v", fullPath, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("Error decoding config file:", err)
	}
}

func proxyHandler(w http.ResponseWriter, r *http.Request) {
	authorization := r.Header.Get("Authorization")
	if len(authorization) == 0 {
		errorMessage := "Authorization header is missing"
		log.Printf("[Error] %s", errorMessage)
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	if len(authorization) < 9 || authorization[:7] != "Bearer " {
		errorMessage := "Invalid Authorization header format"
		log.Printf("[Error] %s", errorMessage)
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	token := authorization[7:]
	log.Printf("Parsed token: %s", token)

	if token[:3] == "ai-" {
		keys, ok := config.Keys[token]
		if !ok {
			log.Printf("[Debug] config.Keys: %v", config.Keys) // 添加此行以显示 config.Keys 的内容
			errorMessage := `{"error":{"message":"Invalid Token","code":403}}`
			log.Printf("[Error] %s", errorMessage)
			http.Error(w, errorMessage, http.StatusForbidden)
			return
		}
		log.Printf("Found keys in config for token %s: %v", token, keys)

		index, _ := keyIndex.LoadOrStore(token, 0)
		r.Header.Set("Authorization", "Bearer "+keys[index.(int)])

		nextIndex := (index.(int) + 1) % len(keys)
		keyIndex.Store(token, nextIndex)

		log.Printf("Used key: %s, Updated index: %d", keys[index.(int)], nextIndex)
	}

	proxyURL := "https://api.openai.com" + r.RequestURI
	req, err := http.NewRequest(r.Method, proxyURL, r.Body)
	if err != nil {
		errorMessage := "Error creating proxy request"
		log.Printf("[Error] %s: %v", errorMessage, err)
		http.Error(w, errorMessage, http.StatusInternalServerError)
		return
	}

	req.Header = r.Header
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		errorMessage := "Error sending proxy request"
		log.Printf("[Error] %s: %v", errorMessage, err)
		http.Error(w, errorMessage, http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	log.Printf("Proxy response: StatusCode=%d, Header=%v", resp.StatusCode, resp.Header)

	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func main() {
	loadConfig()

	http.HandleFunc("/", proxyHandler)
	log.Fatal(http.ListenAndServe(":8124", nil))
}
