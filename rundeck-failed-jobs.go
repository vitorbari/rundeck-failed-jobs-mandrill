package main

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	m "github.com/keighl/mandrill"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

type Recipient struct {
	Email    string
	Name     string
	SendType string
}

type Configuration struct {
	RundeckServerUrl   string
	RundeckApiVersion  string
	RundeckAuthToken   string
	MandrillKey        string
	MandrillFromEmail  string
	MandrillFromName   string
	MandrillRecipients []Recipient
}

type Job struct {
	XMLName     xml.Name `xml:"job"`
	Name        string   `xml:"name"`
	Group       string   `xml:"group"`
	Project     string   `xml:"project"`
	Description string   `xml:"description"`
}

type Node struct {
	XMLName xml.Name `xml:"node"`
	Name    string   `xml:"name,attr"`
}

type FailedNodes struct {
	XMLName xml.Name `xml:"failedNodes"`
	Nodes   []Node   `xml:"node"`
}

type Execution struct {
	XMLName     xml.Name    `xml:"execution"`
	Href        string      `xml:"href,attr"`
	User        string      `xml:"user"`
	Started     string      `xml:"date-started"`
	Ended       string      `xml:"date-ended"`
	Jobs        []Job       `xml:"job"`
	FailedNodes FailedNodes `xml:"failedNodes"`
}

type QueryExecutions struct {
	XMLName    xml.Name    `xml:"executions"`
	Executions []Execution `xml:"execution"`
}

func main() {

	// Path
	dir, err := filepath.Abs(filepath.Dir(os.Args[0]))

	if err != nil {
		log.Fatal(err)
	}

	// flag (Params)
	projectPtr := flag.String("project", "", "the project name")
	groupPtr := flag.String("group", "", "specify a group or partial group path to include all jobs within that group path")
	recentFilterPtr := flag.String("recentfilter", "1h", "Use a simple text format to filter executions that completed within a period of time")

	flag.Parse()

	if len(*projectPtr) == 0 {
		log.Fatal("Missing required [project] param!")
	}

	// Config
	file, err := os.Open(dir + "/conf.json")

	if err != nil {
		log.Fatal(err)
	}

	decoder := json.NewDecoder(file)
	configuration := Configuration{}
	err = decoder.Decode(&configuration)

	// Send get request to Rundeck api
	client := &http.Client{}
	req, err := http.NewRequest("GET", configuration.RundeckServerUrl+"/api/"+configuration.RundeckApiVersion+"/executions?project="+*projectPtr+"&groupPath="+*groupPtr+"&statusFilter=failed&recentFilter="+*recentFilterPtr, nil)
	req.Header.Set("X-Rundeck-Auth-Token", configuration.RundeckAuthToken)
	res, err := client.Do(req)

	if err != nil {
		log.Fatal(err)
	}

	response_body, err := ioutil.ReadAll(res.Body)

	res.Body.Close()

	if err != nil {
		log.Fatal(err)
	}

	var query QueryExecutions
	xml.Unmarshal(response_body, &query)

	failed_executions := len(query.Executions)
	if failed_executions > 0 {

		var buffer bytes.Buffer

		buffer.WriteString(fmt.Sprintf("%v Failed Executions from project [%v]", failed_executions, *projectPtr))

		if len(*groupPtr) != 0 {
			buffer.WriteString(fmt.Sprintf(" group [%v]", *groupPtr))
		}

		buffer.WriteString(".\n\n")

		buffer.WriteString("Executions:\n")

		for _, execution := range query.Executions {

			for _, job := range execution.Jobs {
				buffer.WriteString("\t" + job.Name + "\n")
			}

			buffer.WriteString("\t\t" + execution.Href + "\n")
			buffer.WriteString("\t\tStarted: " + execution.Started + " | User:" + execution.User + "\n")
			buffer.WriteString("\t\tNodes: ")

			for i, node := range execution.FailedNodes.Nodes {
				if i > 0 {
					buffer.WriteString(" / ")
				}
				buffer.WriteString(node.Name)
			}

			buffer.WriteString("\n\n")
		}

		mail_client := m.ClientWithKey(configuration.MandrillKey)

		message := &m.Message{}

		for _, recipient := range configuration.MandrillRecipients {
			message.AddRecipient(recipient.Email, recipient.Name, recipient.SendType)
		}

		message.FromEmail = configuration.MandrillFromEmail
		message.FromName = configuration.MandrillFromName

		if len(*groupPtr) != 0 {
			message.Subject = fmt.Sprintf("[RunDeck] [%v] [%v] %v failures!", *projectPtr, *groupPtr, failed_executions)
		} else {
			message.Subject = fmt.Sprintf("[RunDeck] [%v] %v failures!", *projectPtr, failed_executions)
		}

		message.Text = buffer.String()

		_, err = mail_client.MessagesSend(message)

		if err != nil {
			log.Fatal(err)
		}

		fmt.Println(fmt.Sprintf("%v failed jobs found.", failed_executions))

	} else {
		fmt.Println(fmt.Sprintf("No failed jobs found in the period [%v].", *recentFilterPtr))
	}

	os.Exit(0)

}
