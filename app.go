package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/gdamore/tcell"
	log "github.com/sirupsen/logrus"

	"github.com/rivo/tview"
)

type region struct {
	name string
	description string
}

var (
	mainPage = "*main*"
	regionsList = []region{
		{"af-south-1",  "Africa (Cape Town)"},
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

type App struct {
	config      aws.Config
	instance    *tview.Application
	pages       *tview.Pages
	finderFocus tview.Primitive
	Tag         string
	Query       string
}

func (a *App) Run(writer io.Writer) error {
	a.instance = tview.NewApplication()

	cfg, err := external.LoadDefaultAWSConfig()
	if err != nil {
		log.WithFields(log.Fields{"err": err.Error()}).Fatal("unable to load SDK config")
	}

	a.config = cfg

	a.ecs(a.Tag)

	if err := a.instance.Run(); err != nil {
		log.Fatalf("Error running application: %s\n", err)
	}

	return nil
}

func (a *App) ecs(tag string) {
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
		regions.AddItem(name, region.description, 0, func(){
			instances.Clear()
			clusters.Clear()

			a.config.Region = name

			a.findClusters(clusters, instances)
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
		log.WithFields(log.Fields{ "err": err }).Error("Cluster querying error")
		panic(err)
	} else {
		var count int
		for _, arn := range resp.ClusterArns {
			count += 1
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
		log.WithFields(log.Fields{ "err": err }).Error("Instance querying error")
		panic(err)
	} else {
		if len(resp.ContainerInstanceArns) < 1 {
			a.informational("No instances found in this cluster!")
			return
		} else {
			describe := &ecs.DescribeContainerInstancesInput{
				Cluster: aws.String(cluster),
				ContainerInstances: resp.ContainerInstanceArns,
			}

			req := svc.DescribeContainerInstancesRequest(describe)
			resp, err := req.Send(context.TODO())
			if err != nil {
				log.WithFields(log.Fields{ "err": err }).Error("Unable to describe instances!")
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
		InstanceIds: []string { instance },
	}

	req := svc.DescribeInstancesRequest(input)
	resp, err := req.Send(context.TODO())
	if err != nil {
		log.WithFields(log.Fields{ "err": err }).Error("Instance querying error")
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

	details := tview.NewTable().
		SetFixed(3, 4).
		SetSeparator(tview.Borders.Vertical).
		SetBordersColor(tcell.ColorLightGrey)

	details.SetCell(0, 0, &tview.TableCell{Text: "Private IP", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 1, &tview.TableCell{Text: "Public IP", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 2, &tview.TableCell{Text: "AMI", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1}).
		SetCell(0, 3, &tview.TableCell{Text: "SGs", Align: tview.AlignCenter, Color: tcell.ColorGreen, Expansion: 1})

	details.SetCell(1, 0, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].PrivateIpAddress), Align: tview.AlignLeft, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 1, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].PublicIpAddress), Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 2, &tview.TableCell{Text: aws.StringValue(resp.Reservations[0].Instances[0].ImageId), Align: tview.AlignCenter, Color: tcell.ColorWhite, Expansion: 1}).
		SetCell(1, 3, &tview.TableCell{Text: "sgs", Align: tview.AlignRight, Color: tcell.ColorWhite, Expansion: 1})

	frame := tview.NewFrame(details).
		SetBorders(1, 1, 1, 1, 2, 2).
		AddText(aws.StringValue(resp.Reservations[0].Instances[0].Placement.AvailabilityZone), true, tview.AlignLeft, tcell.ColorWhite).
		AddText(aws.StringValue(instanceDetails.Status), true, tview.AlignCenter, tcell.ColorWhite).
		AddText(string(resp.Reservations[0].Instances[0].InstanceType), true, tview.AlignRight, tcell.ColorWhite).
		AddText("(s) SSH to instance", false, tview.AlignCenter, tcell.ColorDimGrey).
		AddText("(c) Copy private IP", false, tview.AlignCenter, tcell.ColorDimGrey).
		AddText("(p) Copy public IP", false, tview.AlignCenter, tcell.ColorDimGrey).
		AddText("(q) Quit", false, tview.AlignCenter, tcell.ColorDimGrey).
		AddText("(ESC) Back", false, tview.AlignCenter, tcell.ColorDimGrey)

	frame.SetBorder(true).
		SetTitle(fmt.Sprintf(`Instance details "%s"`, instance))


	details.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEscape:
			a.pages.SwitchToPage(mainPage)
			if a.finderFocus != nil {
				a.instance.SetFocus(a.finderFocus)
			}
		case tcell.KeyEnter:
			// overlay with ssh command, or shell ssh to this instance and close
			log.Info("hit enter")
			details.ScrollToEnd()
		case tcell.KeyCtrlQ:

		}
	}).
	SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Rune() {

		case 'q':
			a.instance.Stop()
		case 'c':
			_ = clipboard.WriteAll(aws.StringValue(resp.Reservations[0].Instances[0].PrivateIpAddress))
		case 'p':
			_ = clipboard.WriteAll(aws.StringValue(resp.Reservations[0].Instances[0].PublicIpAddress))
		case 's':
			a.instance.Stop()
			_ = clipboard.WriteAll(fmt.Sprintf("ssh %s", aws.StringValue(resp.Reservations[0].Instances[0].PublicDnsName)))
		}

		return event
	})

	a.pages.AddPage(instance, frame, true, true)
}