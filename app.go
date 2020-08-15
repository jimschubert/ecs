package app

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/gdamore/tcell"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/rivo/tview"
)

type region struct {
	name        string
	description string
}

var (
	mainPage    = "*main*"
	regionsList = []region{
		{"af-south-1", "Africa (Cape Town)"},
		{"ap-east-1", "Asia Pacific (Hong Kong)"},
		{"ap-northeast-1", "Asia Pacific (Tokyo)"},
		{"ap-northeast-2", "Asia Pacific (Seoul)"},
		{"ap-south-1", "Asia Pacific (Mumbai)"},
		{"ap-southeast-1", "Asia Pacific (Singapore)"},
		{"ap-southeast-2", "Asia Pacific (Sydney)"},
		{"ca-central-1", "Canada (Central)"},
		{"eu-central-1", "Europe (Frankfurt)"},
		{"eu-north-1", "Europe (Stockholm)"},
		{"eu-south-1", "Europe (Milan)"},
		{"eu-west-1", "Europe (Ireland)"},
		{"eu-west-2", "Europe (London)"},
		{"eu-west-3", "Europe (Paris)"},
		{"me-south-1", "Middle East (Bahrain)"},
		{"sa-east-1", "South America (Sao Paulo)"},
		{"us-east-1", "US East (N. Virginia)"},
		{"us-east-2", "US East (Ohio)"},
		{"us-west-1", "US West (N. California)"},
		{"us-west-2", "US West (Oregon)"},
	}
)

// App is the structure holding state and inputs for running the application
type App struct {
	config      aws.Config
	instance    *tview.Application
	pages       *tview.Pages
	finderFocus tview.Primitive
	Cluster     string
	Query       string
	PublicKey   string
}

// Run is the running entry point for the application
func (a *App) Run(writer io.Writer) error {
	log.SetOutput(writer)

	a.instance = tview.NewApplication()

	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.WithFields(log.Fields{"err": err.Error()}).Fatal("unable to load SDK config")
	}

	a.config = cfg

	a.ecs()

	if err := a.instance.Run(); err != nil {
		log.Fatalf("Error running application: %s\n", err)
	}

	return nil
}

func (a *App) ecs() {
	regions := tview.NewList().ShowSecondaryText(false)

	clusters := tview.NewList()
	instances := tview.NewList()

	regions.SetBorder(true).SetTitle("Regions")
	clusters.SetBorder(true).SetTitle("Clusters")
	instances.SetBorder(true).SetTitle("Instances")

	clusters.ShowSecondaryText(false).
		SetDoneFunc(func() {
			clusters.Clear()
			instances.Clear()
			a.instance.SetFocus(regions)
		})

	instances.ShowSecondaryText(true).
		SetDoneFunc(func() {
			instances.Clear()
			a.instance.SetFocus(clusters)
		})

	// Create the layout.
	flex := tview.NewFlex().
		AddItem(regions, 20, 0, true).
		AddItem(clusters, 0, 1, false).
		AddItem(instances, 0, 3, false)

	selectedRegion := 0
	defaultRegion, _ := os.LookupEnv("AWS_DEFAULT_REGION")
	for idx, region := range regionsList {
		name := region.name
		regions.AddItem(name, region.description, 0, func() {
			instances.Clear()
			clusters.Clear()

			a.config.Region = name

			if len(a.Cluster) > 0 {
				arr := make([]string, 0)
				a.findClustersByQuery(clusters, instances, nil, &arr)
			} else {
				a.findClusters(clusters, instances)
			}
		})

		if defaultRegion != "" && region.name == defaultRegion {
			selectedRegion = idx
		}
	}

	regions.SetCurrentItem(selectedRegion)

	// Set up the pages and show the Finder.
	a.pages = tview.NewPages().
		AddPage(mainPage, flex, true, true)

	a.instance.SetRoot(a.pages, true)
}

func (a *App) findClusters(clusters *tview.List, instances *tview.List) {
	svc := ecs.New(a.config)
	input := &ecs.ListClustersInput{}
	req := svc.ListClustersRequest(input)
	resp, err := req.Send(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("Cluster querying error")
		panic(err)
	}

	var count int
	for _, arn := range resp.ClusterArns {
		count++
		shortName := strings.SplitAfter(arn, "/")[1]
		clusters.AddItem(shortName, "arn", 0, nil)
	}

	if count < 1 {
		a.informational("No clusters found in this region!")
	} else {

		clusters.SetCurrentItem(0)

		clusters.SetSelectedFunc(func(i int, cluster string, t string, s rune) {
			a.findInstances(cluster, instances)
		})

		a.instance.SetFocus(clusters)
	}
}

func (a *App) findClustersByQuery(clusters *tview.List, instances *tview.List, token *string, arr *[]string) {
	svc := ecs.New(a.config)
	input := &ecs.ListClustersInput{
		NextToken: token,
	}
	req := svc.ListClustersRequest(input)
	resp, err := req.Send(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("Cluster querying error")
		panic(err)
	}

	for _, arn := range resp.ClusterArns {
		shortName := strings.SplitAfter(arn, "/")[1]
		if strings.HasPrefix(shortName, a.Cluster) {
			*arr = append(*arr, arn)
		}
	}

	if resp.NextToken == nil {
		a.displayClusters(clusters, instances, arr)
	} else {
		a.findClustersByQuery(clusters, instances, resp.NextToken, arr)
	}
}

func (a *App) displayClusters(clusters *tview.List, instances *tview.List, arr *[]string) {
	if len(*arr) == 0 {
		a.informational("No instances found in this cluster!")
		return
	}

	for _, arn := range *arr {
		shortName := strings.SplitAfter(arn, "/")[1]
		clusters.AddItem(shortName, "arn", 0, nil)
	}

	clusters.SetCurrentItem(0)

	clusters.SetSelectedFunc(func(i int, cluster string, t string, s rune) {
		a.findInstances(cluster, instances)
	})

	a.instance.SetFocus(clusters)
}

func (a *App) findInstances(cluster string, instances *tview.List) {
	// // Filter on query https://docs.aws.amazon.com/AmazonECS/latest/developerguide/cluster-query-language.html
	svc := ecs.New(a.config)
	input := &ecs.ListContainerInstancesInput{
		Cluster: aws.String(cluster),
	}
	req := svc.ListContainerInstancesRequest(input)
	resp, err := req.Send(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("Instance querying error")
		panic(err)
	} else {
		if len(resp.ContainerInstanceArns) < 1 {
			a.informational("No instances found in this cluster!")
			return
		}
		describe := &ecs.DescribeContainerInstancesInput{
			Cluster:            aws.String(cluster),
			ContainerInstances: resp.ContainerInstanceArns,
		}

		req := svc.DescribeContainerInstancesRequest(describe)
		resp, err := req.Send(context.TODO())
		if err != nil {
			log.WithFields(log.Fields{"err": err}).Error("Unable to describe instances!")
			panic(err)
		}

		detailsMap := make(map[string]ecs.ContainerInstance, 0)

		for _, instanceDetails := range resp.ContainerInstances {
			detailsMap[aws.StringValue(instanceDetails.Ec2InstanceId)] = instanceDetails
			instances.AddItem(
				aws.StringValue(instanceDetails.Ec2InstanceId),
				fmt.Sprintf("  (%s) %d running %d pending; Registered %s",
					aws.StringValue(instanceDetails.Status),
					aws.Int64Value(instanceDetails.RunningTasksCount),
					aws.Int64Value(instanceDetails.PendingTasksCount),
					instanceDetails.RegisteredAt,
				),
				0,
				nil,
			)
		}

		instances.SetCurrentItem(0)

		instances.SetSelectedFunc(func(i int, instance string, t string, s rune) {
			details, found := detailsMap[instance]
			if !found {
				log.Panic("Could not lookup instance details")
			}
			a.instanceDetails(instance, details)
		})

		a.instance.SetFocus(instances)
	}
}

func (a *App) informational(message string) {
	key := "informational"
	if a.pages.HasPage(key) {
		a.pages.RemovePage(key)
	}

	modal := tview.NewModal().
		SetText(message).
		AddButtons([]string{"OK"}).
		SetDoneFunc(func(buttonIndex int, buttonLabel string) {
			a.pages.SwitchToPage(mainPage)
			a.pages.RemovePage(key)
		})

	a.pages.AddPage(key, modal, false, true)
	a.pages.SwitchToPage(key)
}

func (a *App) instanceDetails(instance string, instanceDetails ecs.ContainerInstance) {
	svc := ec2.New(a.config)
	input := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instance},
	}

	req := svc.DescribeInstancesRequest(input)
	resp, err := req.Send(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Error("Instance querying error")
		panic(err)
	}

	a.finderFocus = a.instance.GetFocus()

	// resp.Reservations[0].Instances[0].EbsOptimized
	// resp.Reservations[0].Instances[0].ImageId
	// resp.Reservations[0].Instances[0].LaunchTime
	// resp.Reservations[0].Instances[0].Placement.AvailabilityZone
	// resp.Reservations[0].Instances[0].PrivateDnsName
	// resp.Reservations[0].Instances[0].PrivateIpAddress
	// resp.Reservations[0].Instances[0].PublicDnsName
	// resp.Reservations[0].Instances[0].PublicIpAddress
	// resp.Reservations[0].Instances[0].SecurityGroups[0].GroupId
	// resp.Reservations[0].Instances[0].State.Name
	// resp.Reservations[0].Instances[0].SubnetId
	// resp.Reservations[0].Instances[0].VpcId
	// resp.Reservations[0].Instances[0].Tags[0].Key, resp.Reservations[0].Instances[0].Tags[0].Value

	if a.pages.HasPage(instance) {
		a.pages.SwitchToPage(instance)
		return
	}

	textView := func(text string, color tcell.Color) tview.Primitive {
		return tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetText(text).
			SetTextColor(color)
	}

	header := func(text string) tview.Primitive {
		return textView(text, tcell.ColorDarkGreen)
	}

	footer := func(text string) tview.Primitive {
		return textView(text, tcell.ColorDimGrey)
	}

	details := tview.NewTable().
		SetFixed(3, 4).
		SetSeparator(tview.Borders.Vertical).
		SetBordersColor(tcell.ColorLightGrey)

	details.SetCell(0, 0, &tview.TableCell{Text: "Private IP", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 1, &tview.TableCell{Text: "Public IP", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 2, &tview.TableCell{Text: "AMI", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 3, &tview.TableCell{Text: "SGs", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1})

	details.SetCell(1, 0, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].PrivateIpAddress), Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 1, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].PublicIpAddress), Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 2, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].ImageId), Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 3, &tview.TableCell{Text: "sgs", Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1})

	grid := tview.NewGrid().
		SetRows(3, 0, 1).
		SetColumns(20, 0, 0, 0, 20).
		SetBorders(false).
		AddItem(header(aws.StringValue(resp.Reservations[0].Instances[0].Placement.AvailabilityZone)), 0, 0, 1, 1, 0, 0, false).
		AddItem(header(aws.StringValue(instanceDetails.Status)), 0, 2, 1, 1, 0, 0, false).
		AddItem(header(string(resp.Reservations[0].Instances[0].InstanceType)), 0, 4, 1, 1, 0, 0, false).
		AddItem(details, 1, 0, 1, 5, 0, 0, false).
		AddItem(footer("(s) SSH"), 2, 0, 1, 1, 0, 0, false).
		AddItem(footer("(c) Copy private IP"), 2, 1, 1, 1, 0, 0, false).
		AddItem(footer("(p) Copy public IP"), 2, 2, 1, 1, 0, 0, false).
		AddItem(footer("(ESC) Back"), 2, 3, 1, 1, 0, 0, false).
		AddItem(footer("(q) Quit"), 2, 4, 1, 1, 0, 0, false)

	grid.
		SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
			if event.Key() == tcell.KeyESC {
				a.pages.SwitchToPage(mainPage)
				if a.finderFocus != nil {
					a.instance.SetFocus(a.finderFocus)
				}
			} else if event.Key() == tcell.KeyEnter {
				// overlay with ssh command, or shell ssh to this instance and close
				log.Info("hit enter")
				details.ScrollToEnd()
			} else {
				switch event.Rune() {

				case 'q':
					a.instance.Stop()
				case 'c':
					_ = clipboard.WriteAll(aws.StringValue(resp.Reservations[0].Instances[0].PrivateIpAddress))
				case 'p':
					_ = clipboard.WriteAll(aws.StringValue(resp.Reservations[0].Instances[0].PublicIpAddress))
				case 's':
					panic("Not implemented yet…")
					// _, session, err := a.connectSSH(aws.StringValue(resp.Reservations[0].Instances[0].PublicIpAddress), a.PublicKey)
					// if err != nil {
					// 	panic(err)
					// }
					// if session != nil {
					//
					// 	a.instance.Suspend(func(){
					// 		// start shell
					// 		if err := session.Shell(); err != nil {
					// 			log.Errorf("Couldn't start shell: %v", err)
					// 			return
					// 		}
					//
					// 		session.Stdout = os.Stdout
					// 		session.Stderr = os.Stderr
					// 		session.Stdin = os.Stdin
					//
					// 		session.Wait()
					// 		defer session.Close()
					// 	})
					// }
				}
			}

			return event
		})

	frame := tview.NewFrame(grid)

	frame.
		SetBorder(true).
		SetTitle(fmt.Sprintf(`Instance details "%s"`, instance))

	a.pages.AddPage(instance, frame, true, true)
}

func (a *App) connectSSH(location string, key string) (*ssh.Client, *ssh.Session, error) {
	// todo; include ssh agent as an auth method
	clientConfig := &ssh.ClientConfig{
		User: "ec2-user",
		Auth: []ssh.AuthMethod{
			publicKey(key),
		},
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			return nil
		},
	}

	clientConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()

	// todo; support custom ssh port?
	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", location), clientConfig)
	if err != nil {
		return nil, nil, err
	}

	session, err := client.NewSession()
	if err != nil {
		defer client.Close()
		return nil, nil, err
	}

	return client, session, nil
}

func publicKey(location string) ssh.AuthMethod {
	fullPath := location
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		home, err := os.UserHomeDir()
		if err != nil {
			panic(err)
		}
		fullPath = path.Join(home, ".ssh", location)
	}

	key, err := ioutil.ReadFile(fullPath)
	if err != nil {
		panic(err)
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		panic(err)
	}
	return ssh.PublicKeys(signer)
}
