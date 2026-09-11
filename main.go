package main

import ("fmt"
 "net/url"
 "net/http"
  "net/http/httputil"
"sync"
"time"
"net")

type Backend struct {
	URL *url.URL
	Alive bool 
	Proxy *httputil.ReverseProxy
	mux sync.RWMutex
}
type ServerPool struct {
	backends []*Backend
	current int
	mux sync.Mutex

}
func (b *Backend) SetAlive( alive bool){
	b.mux.Lock()
	b.Alive = alive
	defer b.mux.Unlock()
}
func (b *Backend) IsAlive() bool {
	b.mux.RLock()
	alive := b.Alive
	b.mux.RUnlock()
	return alive
}
func (s *ServerPool) NextIndex() int{
	s.mux.Lock()
	s.current = (s.current+1)% len(s.backends)
	defer s.mux.Unlock()

	return s.current
}
func (s *ServerPool) AddBackend( b *Backend){
	s.backends = append(s.backends, b)
}
func (s *ServerPool) GetNextPeer() *Backend{
	for i :=0; i< len(s.backends); i++{
		index := s.NextIndex()
		if(s.backends[index].IsAlive()){
			return s.backends[index]
		}
	}
	return nil
}
func HealthCheck(serverPool *ServerPool){
	t := time.NewTicker(time.Second*10)
	for range t.C{
		println("Starting health check...")
		for _, b := range serverPool.backends{
			conn, err := net.DialTimeout("tcp", b.URL.Host,time.Second*2)
			if(err !=nil){
				b.SetAlive(false)
				fmt.Printf("Backend is down: %s\n", b.URL.Host)
			}else{
				conn.Close()
				b.SetAlive(true)
			}


		}
	}

 }
func main(){

	var serverPool ServerPool
	serverList := []string{"http://localhost:8081", "http://localhost:8082", "http://localhost:8083"}
	
	for _, backend := range serverList {
		parseUrl, err := url.Parse(backend)
		if err != nil {
			fmt.Printf("Error parsing URL: %v\n", err)
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(parseUrl)
		backendObj := &Backend{URL: parseUrl, Alive: true, Proxy: proxy}
		serverPool.AddBackend(backendObj)
	
	}

		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request){
		   nextPeer := serverPool.GetNextPeer()
		   if(nextPeer == nil){
			http.Error(w, "All Servers are down", http.StatusServiceUnavailable)
			return
		   }
		   fmt.Printf("Forwarding request to %s\n", nextPeer.URL)
		   nextPeer.Proxy.ServeHTTP(w, r)
		})
		go HealthCheck(&serverPool)

		http.ListenAndServe(":8080", nil)
}