package main

import (
	"github.com/elazarl/go-bindata-assetfs"
	"github.com/yoink00/simpleawsmgr/assets"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"

	"github.com/gorilla/websocket"
)

var cfg *aws.Config
var sess *session.Session

func init() {
	cfg = aws.NewConfig()
	sess = session.Must(session.NewSession(cfg))
}

type EC2Instance struct {
	Name          string
	InstanceID    string
	PublicIP      string
	PrivateIP     string
	IsDevelopment bool
	IsBackedUp    bool
	State         string
}

type EC2Action struct {
	Action     string
	InstanceID string
}

func (i *EC2Instance) String() string {
	return fmt.Sprintf(
		"%s (public IP: %s, private IP: %s) is %sbacked up. Instance ID: %s",
		i.Name,
		i.PublicIP,
		i.PrivateIP,
		func() string {
			if i.IsBackedUp {
				return ""
			}
			return "not "
		}(),
		i.InstanceID)
}

func (i *EC2Instance) IsDiff(i2 *EC2Instance) bool {

	if i.Name != i2.Name {
		return true
	}

	if i.InstanceID != i2.InstanceID {
		return true
	}

	if i.PublicIP != i2.PublicIP {
		return true
	}

	if i.PrivateIP != i2.PrivateIP {
		return true
	}

	if i.IsDevelopment != i2.IsDevelopment {
		return true
	}

	if i.IsBackedUp != i2.IsBackedUp {
		return true
	}

	if i.State != i2.State {
		return true
	}

	return false
}

func pollForAwsEC2State(instanceChannel chan<- *EC2Instance) {
	defer close(instanceChannel)

	ec2svc := ec2.New(sess)

	filter := ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			&ec2.Filter{
				Name: aws.String("tag:Development"),
				Values: []*string{
					aws.String("True"),
				},
			},
		},
	}

	for {
		res, err := ec2svc.DescribeInstances(&filter)
		if err != nil {
			println("Error: ", err.Error())
			break
		}

		for _, r := range res.Reservations {
			for _, i := range r.Instances {
				instance := new(EC2Instance)
				instance.InstanceID = *i.InstanceId
				instance.PrivateIP = *i.PrivateIpAddress
				instance.State = *i.State.Name
				if i.PublicIpAddress != nil {
					instance.PublicIP = *i.PublicIpAddress
				}
				for _, t := range i.Tags {
					switch {
					case *t.Key == "Development" && *t.Value == "True":
						instance.IsDevelopment = true
					case *t.Key == "Backup" && *t.Value == "True":
						instance.IsBackedUp = true
					case *t.Key == "Name":
						instance.Name = *t.Value
					}
				}

				log.Println("Publishing: ", instance.String())
				instanceChannel <- instance
			}
		}
	 	time.Sleep(10 * time.Second)
	}
}

func ec2ActionHandler(actionChannel <-chan *EC2Action) {
	ec2svc := ec2.New(sess)

	for action := range actionChannel {
		if action.Action == "stop" {
			log.Println("Stopping ", action.InstanceID)
			inp := ec2.StopInstancesInput{}
			inp.InstanceIds = append(inp.InstanceIds, &action.InstanceID)
			_, err := ec2svc.StopInstances(&inp)
			if err != nil {
				log.Println("Unable to stop instance", action.InstanceID, err)
			}
		} else if action.Action == "start" {
			log.Println("Starting ", action.InstanceID)
			inp := ec2.StartInstancesInput{}
			inp.InstanceIds = append(inp.InstanceIds, &action.InstanceID)
			_, err := ec2svc.StartInstances(&inp)
			if err != nil {
				log.Println("Unable to start instance", action.InstanceID, err)
			}
		}
	}
}

func publisher(registerChannel <-chan *ec2UpdateChannel, instanceChannel <-chan *EC2Instance) {

	subs  := make(map[int]*ec2UpdateChannel)
	instanceMap := make(map[string]*EC2Instance, 10)

	// Close all the open channels
	defer func() {
		for _, c := range subs {
			close(c.channel)
		}
	}()

	outer:
	for {
		select {
		case newSub, ok := <-registerChannel:
			if !ok {
				log.Println("Register channel is closed")
				break outer
			}

			// We already know about this subscriber.
			// We will remove them from our subscriber list.
			if s, ok := subs[newSub.id]; ok {
				log.Printf("Removing %d from subscriber list", newSub.id)
				close(s.channel)
				delete(subs, newSub.id)
				continue
			}

			// Add sub to sub map
			log.Printf("Adding %d to subscriber list", newSub.id)
			subs[newSub.id] = newSub

			for _, v := range instanceMap {
				newSub.channel <- v
			}
		case upd, ok := <-instanceChannel:
			if !ok {
				log.Println("Instance poller channel is closed")
				break outer
			}

			if ins, ok := instanceMap[upd.InstanceID]; ok && !ins.IsDiff(upd) {
				log.Println("Received update but it is not different from previous update")
				continue
			}
			log.Println("Publishing update: ", upd.String())
			instanceMap[upd.InstanceID] = upd
			for _, s := range subs {
				s.channel <- upd
			}
		}
	}

	log.Println("Stopped publisher")
}

type ec2UpdateChannel struct {
	channel chan *EC2Instance
	id      int
}

type LoggingHttpFileSystem struct {
	fs http.FileSystem
}

func (lfs *LoggingHttpFileSystem) Open(filename string) (http.File, error) {
	log.Println("Fetching", filename)
	file, err := lfs.fs.Open(filename)
	if err != nil {
		log.Printf("Error opening %s: %#v", filename, err)
	}

	return file, err
}

func NewLoggingHttpFileSystem(fs http.FileSystem) http.FileSystem {
	newFs := new(LoggingHttpFileSystem)
	newFs.fs = fs

	return newFs 
}

func main() {

	instanceChannel := make(chan *EC2Instance, 10)

	registerChannel := make(chan *ec2UpdateChannel, 10)
	defer close(registerChannel)

	actionChannel := make(chan *EC2Action, 10)

	go publisher(registerChannel, instanceChannel)
	go pollForAwsEC2State(instanceChannel)
	go ec2ActionHandler(actionChannel)

	upgrader := websocket.Upgrader{}
	upgrader.CheckOrigin = func(request *http.Request) bool {
		return true
	}

	var ec2InstanceChannelID int

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Unable to upgrade: ", err)
			return
		}

		ec2Updates := &ec2UpdateChannel{}
		ec2Updates.channel = make(chan *EC2Instance, 10)
		ec2Updates.id = ec2InstanceChannelID
		ec2InstanceChannelID++
		registerChannel <- ec2Updates

		go func() {
			for {
				var action EC2Action
				err := conn.ReadJSON(&action)
				if err != nil {
					log.Println("Error reading from connection: ", err)
					registerChannel <- ec2Updates
					break
				}
				log.Printf("Got action: %#v", action)
				actionChannel <- &action
			}

			log.Println("Exiting read handler")
		}()

		for u := range ec2Updates.channel {
			err := conn.WriteJSON(*u)
			if err != nil {
				log.Println("Unable send ec2Update: ", err)
				return
			}
		}

		log.Println("Stopped handling client")
	})

	http.Handle("/", http.FileServer(NewLoggingHttpFileSystem(&assetfs.AssetFS{
		Asset: assets.Asset,
	    AssetDir: assets.AssetDir,
	    AssetInfo: assets.AssetInfo,
		Prefix: "/",
	})))

	log.Fatal(http.ListenAndServe("localhost:19780", nil))
}
