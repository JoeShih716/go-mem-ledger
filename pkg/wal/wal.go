package wal

import (
	"encoding/json"
	"io/fs"
	"os"
	"sync"
)

// 自己定義常用的權限常量
const (
	// rw-r--r-- (擁有者讀寫，其他人唯讀) - 適用於大多數檔案
	FileModeReadOnly fs.FileMode = 0644

	// rwxr-xr-x (擁有者全開，其他人可讀可執行) - 適用於執行檔或 Script
	FileModeExecutable fs.FileMode = 0755

	// rw------- (只有擁有者可讀寫) - 適用於私鑰、機密檔
	FileModePrivate fs.FileMode = 0600
)

type WAL struct {
	file *os.File
	mu   sync.Mutex
}

// NewWAL 開啟或建立一個 WAL 檔案
// O_RDWR讀寫模式
// O_APPEND 每次寫入時自動跳到文件末尾
// O_CREATE 如果文件不存在則建立
func NewWAL(path string) (*WAL, error) {
	// 提示: os.OpenFile with O_APPEND|O_CREATE|O_RDWR
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_RDWR, FileModeReadOnly)
	if err != nil {
		return nil, err
	}
	return &WAL{file: file,
		mu: sync.Mutex{},
	}, nil
}

// Write 寫入一筆資料
func (w *WAL) Write(v any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := json.NewEncoder(w.file).Encode(v); err != nil {
		return err
	}
	return w.file.Sync()
}

// Sync 強制刷入硬碟 (關鍵！)
func (w *WAL) Sync() error {
	return w.file.Sync()
}

// Close 關閉檔案
func (w *WAL) Close() error {
	return w.file.Close()
}

// ReadAll 讀取所有資料
// callback 是一個函式，接收一個 json.RawMessage
// 這樣可以避免一次將所有資料載入記憶體
func (w *WAL) ReadAll(callback func(jsonRaw []byte) error) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 確保從頭讀取
	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}

	decoder := json.NewDecoder(w.file)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err.Error() == "EOF" { // io.EOF check
				break
			}
			return err
		}
		if err := callback(raw); err != nil {
			return err
		}
	}
	return nil
}
