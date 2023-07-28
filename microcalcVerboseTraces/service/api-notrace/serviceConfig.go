package apinotrace

// import (
// 	"log"
// 	"os"
// 	"path/filepath"
//
// 	"gopkg.in/yaml.v2"
// )

// type Config struct {
// 	Services []struct {
// 		Name string `yaml:"name"`
// 		Host string `yaml:"host"`
// 		Port int    `yaml:"port"`
// 	} `yaml:"services"`
// }

// func GetServices() Config {
// 	wd, err := os.Getwd()
// 	if err != nil {
// 		log.Println("Error:", err)
// 	}
//
// 	relativePath := "../../services.yaml"
//
// 	absolutePath := filepath.Join(wd, relativePath)
//
// 	f, err := os.Open(absolutePath)
// 	if err != nil {
// 		log.Fatal("could not open config")
// 	}
// 	defer f.Close()
//
// 	var cfg Config
// 	decoder := yaml.NewDecoder(f)
// 	err = decoder.Decode(&cfg)
// 	if err != nil {
// 		log.Fatal("could not process config")
// 	}
// 	return cfg
// }
