package main

import (
	"fmt"
	"os"
	"strings"

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
	hostnames := []IMSResponse{}
	for i := 0; i < 100; i++ {
		server := IMSResponse{
			Dbms:     "oracle",
			Hostname: fmt.Sprintf("host-%d", i),
		}
		hostnames = append(hostnames, server)
	}
	sp := selection.New("Select hostnames", hostnames)
	sp.FilterPrompt = "Filter by ID:"
	sp.FilterPlaceholder = "Type to filter"
	sp.PageSize = 3
	sp.LoopCursor = true
	sp.Filter = func(filter string, choice *selection.Choice[IMSResponse]) bool {
		return strings.HasPrefix(choice.Value.Hostname, filter)
	}
	sp.ExtendedTemplateFuncs = map[string]interface{}{
		"name": func(c *selection.Choice[IMSResponse]) string { return c.Value.Hostname },
	}
	sp.PageSize = 10

	choice, err := sp.RunPrompt()
	if err != nil {
		fmt.Printf("Error: %v\n", err)

		os.Exit(1)
	}

	// do something with the final choice
	fmt.Println(choice)
}
