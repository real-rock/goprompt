package main

import (
	"fmt"
	"os"

	"github.com/real-rock/goprompt/selection"
)

type IMSResponse struct {
	Dbms       string `json:"dbms"`
	Dba        string `json:"dba"`
	ZoneName   string `json:"zone_name"`
	IP         string `json:"ip"`
	DbCellName string `json:"db_cell_name"`
	ServerType string `json:"server_type"`
	Hostname   string `json:"hostname"`
	OsName     string `json:"os_name"`
	Domain     string `json:"domain"`
}

func main() {
	hostnames := []string{}
	for i := 0; i < 500000; i++ {
		hostnames = append(hostnames, fmt.Sprintf("host-%d", i))
	}
	sp := selection.New("Select hostnames", hostnames)
	sp.FilterPrompt = "Filter by ID:"
	sp.FilterPlaceholder = "Type to filter"
	sp.LoopCursor = true
	sp.PageSize = 10

	choice, err := sp.RunPrompt()
	if err != nil {
		fmt.Printf("Error: %v\n", err)

		os.Exit(1)
	}

	// do something with the final choice
	fmt.Println(choice)
}
