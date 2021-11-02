package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

var (
	sidecarConfig *SidecarConfig
	sidecarError  error
)

func main() {
	// 定义参数
	var parameters WebHookParameters
	flag.IntVar(&parameters.port, "port", 8088, "webhook server port")
	flag.StringVar(&parameters.certFile, "tlsCertFile", "D:\\code\\go\\src\\github.com\\winkyi\\mutating-webhook-demo\\config\\webhook-server-tls.crt", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&parameters.keyFile, "tlsKeyFile", "D:\\code\\go\\src\\github.com\\winkyi\\mutating-webhook-demo\\config\\webhook-server-tls.key", "File containing the x509 private key to --tlsCertFile.")
	flag.StringVar(&parameters.sidecarConfigFile, "sidecarConfigFile", "D:\\code\\go\\src\\github.com\\winkyi\\mutating-webhook-demo\\config\\sidecar-template.yaml", "File containing the mutation configuration.")
	flag.Parse()

	sidecarConfig, sidecarError = loadSidecarConfig(parameters.sidecarConfigFile)
	if sidecarError != nil {
		log.Println("读取sidecar配置文件失败")
		panic(sidecarError)
	}

	log.Println(sidecarConfig)

	mux := http.NewServeMux()
	mux.Handle("/mutate", admitFuncHandler(addNginxSidecar))
	server := &http.Server{
		Addr:    fmt.Sprintf(":%v", parameters.port),
		Handler: mux,
	}

	log.Fatal(server.ListenAndServeTLS(parameters.certFile, parameters.keyFile))

}
