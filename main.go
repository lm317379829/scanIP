package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

//go:embed static/index.html
var indexHTML embed.FS

func handeleMain(w http.ResponseWriter, req *http.Request) {
	if req.URL.RawQuery == "" {
		// 如果没有查询参数，则返回 index.html 的内容

		// 获取嵌入的 index.html 文件
		index, err := indexHTML.Open("static/index.html")
		if err != nil {
			http.Error(w, "内部服务器错误", 500)
			return
		}
		defer index.Close()

		// 将嵌入的文件内容复制到响应中
		io.Copy(w, index)
	} else {
		query := req.URL.Query()
		ips := query.Get("ips")
		port := query.Get("port")
		if ips == "" || port == "" {
			http.Error(w, "缺少ips或port参数", 404)
			return
		}
		dotCount := strings.Count(ips, ".")
		if dotCount >= 3 {
			parts := strings.Split(strings.Trim(ips, "."), ".")
			ips = fmt.Sprintf("%s.%s.%s", parts[0], parts[1], parts[2])
		}

		// 清空 urlList
		urlList := make([]string, 0, 256)
		for i := 0; i < 256; i++ {
			url := fmt.Sprintf("http://%s.%d:%s", ips, i, port)
			urlList = append(urlList, url)
		}

		found := make(chan string, 1)
		ctx, cancel := context.WithCancel(context.Background())
		// ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		var wg sync.WaitGroup
		client := http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
			},
		}
		header := map[string]string{
			"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/94.0.4606.54 Safari/537.36",
		}

		for _, url := range urlList {
			wg.Add(1)
			retry := 0
			url := fmt.Sprintf("%s/gb.asp", url)
			go func(url string) {
				defer wg.Done()
				for retry < 5 {
					// req, err := http.NewRequest("GET", url, nil)
					req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
					if err != nil {
						log.Infof("%s 访问创建失败: %v", url, err)
						retry++
						continue
					}
					for key, value := range header {
						req.Header.Set(key, value)
					}
					// req = req.WithContext(ctx)

					resp, err := client.Do(req)
					if err == nil && resp.StatusCode == http.StatusOK {
						select {
						case found <- url:
							cancel() // 找到结果后取消所有请求
						default:
						}
						log.Infof("%s 访问成功", url)
					} else {
						retry++
						// log.Infof("%s 访问失败: %v", url, err)
					}
				}

			}(url)
		}

		go func() {
			wg.Wait()
			close(found)
		}()

		select {
		case url := <-found:
			if url != "" {
				url = strings.Trim(url, "/gb.asp")
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(url))
			} else {
				http.Error(w, "未找到有效结果", 404)
				// w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				// w.WriteHeader(http.StatusNotFound)
				// w.Write([]byte("未找到有效结果"))
			}
		case <-ctx.Done():
			http.Error(w, "未找到有效结果", 404)
			// w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			// w.WriteHeader(http.StatusNotFound)
			// w.Write([]byte("未找到有效结果"))
		}
	}
}

func main() {
	// 定义命令行参数
	port := flag.String("port", "10079", "Service's port")

	// 解析命令行参数
	flag.Parse()

	// 设置日志输出
	log.SetOutput(os.Stdout)
	log.SetLevel(log.InfoLevel)

	s := http.Server{
		Addr:    ":" + *port,
		Handler: http.HandlerFunc(handeleMain),
	}
	s.SetKeepAlivesEnabled(false)
	s.ListenAndServe()
}
