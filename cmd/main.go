package main

import (
	"log"

	"github.com/gin-gonic/gin"
)

func main() {
	router := gin.Default()

	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "metro",
		})
	})

	router.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"name":    "Metro",
			"version": "0.0.1",
		})
	})

	log.Println("Metro API starting on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatal(err)
	}
}
