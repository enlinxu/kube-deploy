/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"flag"
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/apiserver/pkg/util/logs"
	"k8s.io/kube-deploy/cluster-api/tools/nodescaler/util"
	clusterapiutil "k8s.io/kube-deploy/cluster-api/util"
)

type ScaleOptions struct {
	scaleOut   bool
	kubeConfig string
	nodeName   string
	verbo      string
}

var ro = &ScaleOptions{}

var rootCmd = &cobra.Command{
	Use:   "nodescaler",
	Short: "Scale node",
	Long:  `Scales given node`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := RunScale(ro); err != nil {
			glog.Exit(err)
		}
	},
}

func RunScale(ro *ScaleOptions) error {
	// set the "log verbosity" flag
	vString := "-v=" + ro.verbo
	flag.CommandLine.Parse([]string{vString})

	// resolve kube-config path
	if ro.kubeConfig == "" {
		ro.kubeConfig = clusterapiutil.GetDefaultKubeConfigPath()
	}

	// create client for core kubernetes API
	kubeClient, err := clusterapiutil.NewKubernetesClient(ro.kubeConfig)
	if err != nil {
		err = fmt.Errorf("error creating kube client set: %v", err)
		return err
	}

	// create client for Cluster API
	c, err := clusterapiutil.NewClientSet(ro.kubeConfig)
	if err != nil {
		err = fmt.Errorf("error creating Cluster API client set: %v", err)
		return err
	}

	//
	// Test entities
	//

	actionItemDTO := ro.nodeName
	tracker := "replace this string with tracker object when integrated"
	var executor util.ScaleActionExecutor
	if ro.scaleOut {
		executor, err = util.NewNodeProvisioner(c, kubeClient)
	} else {
		executor, err = util.NewNodeSuspender(c, kubeClient)
	}
	if err != nil {
		return err
	}

	// TODO: When integrating with kubeturbo:
	// TODO:   Consider using a switch based on ActionExecutor type to completely ignore the kubeturbo-based
	// TODO:   TurboExecutor code from the very outset, even for the action keep alive and action locking.

	//
	// Action keepAlive: Create progress indicator to send frequent updates to Turbo server to prevent Turbo timeout
	//

	// create progress indicator
	progress := util.NewProgress(tracker)

	// start keepAlive
	stop := make(chan struct{})
	defer close(stop)
	go progress.KeepAlive(util.ActionKeepAliveInterval, stop)

	//
	// Action lock: Lock the action to the ScalingController
	//

	// identify the Node and its ScalingController and get the Controller's key with which to lock the action
	progress.Update(0, "Identifying ScalingController managing the Node", util.ActionLockAcquisition)
	lockKey, err := util.GetLockKey(executor, actionItemDTO, progress)
	if err != nil {
		return err
	}

	// TODO: When integrating with kubeturbo:
	// TODO:   1. Add nodeLockStore to ActionHandler struct
	// TODO:   2. Add new signature to IActionLockStore interface:
	// TODO:         getLockWithKey(key string) (*util.LockHelper, error)
	// TODO:   2. Implement ActionLockStore.getLockWithKey(key string) modeled after
	// TODO:      ActionLockStore.getLock(actionItem *proto.ActionItemDTO)
	// TODO:   3. Modify ActionHandler.execute(): When executor type is a ScalingExecutor:
	// TODO:         lockKey := util.GetLockKey(executor, actionItemDTO, progress)
	// TODO:         getLockWithKey(lockKey)

	// lock scaling action to the ScalingController to prevent concurrent scaling actions
	progress.Update(0, fmt.Sprintf("Acquiring Action lock: key=\"%s\" (ScalingController)", lockKey), util.ActionLockAcquisition)
	util.AcquireLock(lockKey)
	if err != nil {
		return err
	}
	defer util.ReleaseLock(lockKey)   // release lock when Action Execution terminates for any reason

	//
	// Execute Scaling Action: Call ScaleActionExecutor.Execute()
	//

	// execute scaling action
	// TODO: Either:
	// TODO:   1. Modify kubeturbo TurboActionExecutor.Execute() method, adding progress indicator argument
	// TODO:   2. Create kubeturbo TurboNodeActionExecutor.Execute() method, adding progress indicator argument
	//_, err = r.Execute(actionItemDTO, progress)
	//_, err = r.ExecuteWithProgress(actionItemDTO, progress)
	switch executor := executor.(type) {
	case *util.Provisioner:
		_, err = executor.ExecuteWithProgress(actionItemDTO, progress, lockKey)
	case *util.Suspender:
		_, err = executor.ExecuteWithProgress(actionItemDTO, progress, lockKey)
	default:
		glog.Error("Unsupported executor type: %s", executor)
	}
	time.Sleep(1 * time.Second)
	return err
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		glog.Exit(err)
	}
}

func init() {
	rootCmd.Flags().BoolVarP(&ro.scaleOut, "scaleout", "s", true, "scaling direction: out (true) or in (false)")
	rootCmd.PersistentFlags().StringVarP(&ro.kubeConfig, "kubeconfig", "k", "", "location of kubernetes config file (default $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&ro.nodeName, "nodename", "n", "", "node to be cloned")
	rootCmd.PersistentFlags().StringVarP(&ro.verbo, "verbo", "b", "1", "verbo level")
	//flag.CommandLine.Parse([]string{})
	//flag.CommandLine.Parse([]string{"-v=2"})
	logs.InitLogs()
}
