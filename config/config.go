package config

import "time"

// Config 结构体用于存放所有应用配置
type Config struct {
	ChirpStackServer string
	APIToken         string
	StatusServerURL  string
	ListenAddress    string
	GRPCTimeout      time.Duration
	HTTPTimeout      time.Duration
	MulticastGroups  map[string]string
}

// LoadConfig 加载并返回配置
func LoadConfig() Config {
	return Config{
		ChirpStackServer: "49.232.192.237:18080",
		APIToken:         "eyJ0eXAiOiJKV1QiLCJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJjaGlycHN0YWNrIiwiaXNzIjoiY2hpcnBzdGFjayIsInN1YiI6IjQyOTVmNTUxLTU5YzEtNGIwOS1iMmRhLTBkNjFmYTQ2YmI1NiIsInR5cCI6ImtleSJ9.cgiNxrWfEuPjgwHOQs6t_wrXzH0q7vC_NoN42Y68r4Q",
		StatusServerURL:  "http://111.20.150.242:10088",
		ListenAddress:    "0.0.0.0:10088",
		GRPCTimeout:      5 * time.Second,
		HTTPTimeout:      5 * time.Second,
		MulticastGroups: map[string]string{
			"group1": "e81cd77b-f1e9-40fc-87ba-10e1fc935596",
			"group2": "d696d6eb-24d1-412c-a504-7a57acb2195e",
		},
	}
}
