/*
Copyright 2024 The Kubernetes Authors.

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

package new_controllers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	nfdv1 "sigs.k8s.io/node-feature-discovery-operator/api/v1"
)

var _ = Describe("Reconcile", func() {
	var (
		ctrl       *gomock.Controller
		mockHelper *MocknodeFeatureDiscoveryHelperAPI
		nfdr       *nodeFeatureDiscoveryReconciler
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockHelper = NewMocknodeFeatureDiscoveryHelperAPI(ctrl)

		nfdr = &nodeFeatureDiscoveryReconciler{
			helper: mockHelper,
		}
	})

	ctx := context.Background()

	It("good flow without finalization", func() {
		nfdCR := nfdv1.NodeFeatureDiscovery{}

		mockHelper.EXPECT().hasFinalizer(&nfdCR).Return(true)
		mockHelper.EXPECT().handleMaster(ctx, &nfdCR).Return(nil)
		mockHelper.EXPECT().handleWorker(ctx, &nfdCR).Return(nil)
		mockHelper.EXPECT().handleTopology(ctx, &nfdCR).Return(nil)
		mockHelper.EXPECT().handleGC(ctx, &nfdCR).Return(nil)
		mockHelper.EXPECT().handlePrune(ctx, &nfdCR).Return(nil)
		mockHelper.EXPECT().handleStatus(ctx, &nfdCR).Return(nil)

		res, err := nfdr.Reconcile(ctx, &nfdCR)
		Expect(res).To(Equal(reconcile.Result{}))
		Expect(err).To(BeNil())
	})

	DescribeTable("finalization flow", func(finalizationError error) {
		nfdCR := nfdv1.NodeFeatureDiscovery{}
		timestamp := metav1.Now()
		nfdCR.SetDeletionTimestamp(&timestamp)
		mockHelper.EXPECT().finalizeComponents(ctx, &nfdCR).Return(finalizationError)

		res, err := nfdr.Reconcile(ctx, &nfdCR)
		Expect(res).To(Equal(reconcile.Result{}))
		if finalizationError != nil {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("finalization failed", fmt.Errorf("finalization error")),
		Entry("finalization succeeded", fmt.Errorf("finalization error")),
	)

	DescribeTable("setFinalizer flow", func(setFinalizerError error) {
		nfdCR := nfdv1.NodeFeatureDiscovery{}
		mockHelper.EXPECT().hasFinalizer(&nfdCR).Return(false)
		mockHelper.EXPECT().setFinalizer(ctx, &nfdCR).Return(setFinalizerError)

		res, err := nfdr.Reconcile(ctx, &nfdCR)
		Expect(res).To(Equal(reconcile.Result{}))
		if setFinalizerError != nil {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("setFinalizer failed", fmt.Errorf("set finalizer error")),
		Entry("setFinalizer succeeded", fmt.Errorf("set finalizer error")),
	)

	DescribeTable("check components error flows", func(handlerMasterError,
		handlerWorkerError,
		handleTopologyError,
		handlerGCError,
		handlePruneError,
		handleStatusError error) {
		nfdCR := nfdv1.NodeFeatureDiscovery{}

		mockHelper.EXPECT().hasFinalizer(&nfdCR).Return(true)
		mockHelper.EXPECT().handleMaster(ctx, &nfdCR).Return(handlerMasterError)
		mockHelper.EXPECT().handleWorker(ctx, &nfdCR).Return(handlerWorkerError)
		mockHelper.EXPECT().handleTopology(ctx, &nfdCR).Return(handleTopologyError)
		mockHelper.EXPECT().handleGC(ctx, &nfdCR).Return(handlerGCError)
		mockHelper.EXPECT().handlePrune(ctx, &nfdCR).Return(handlePruneError)
		mockHelper.EXPECT().handleStatus(ctx, &nfdCR).Return(handleStatusError)

		res, err := nfdr.Reconcile(ctx, &nfdCR)
		Expect(res).To(Equal(reconcile.Result{}))
		if handlerMasterError != nil || handlerWorkerError != nil || handleTopologyError != nil ||
			handlerGCError != nil || handlePruneError != nil || handleStatusError != nil {
			Expect(err).To(HaveOccurred())
		} else {
			Expect(err).To(BeNil())
		}
	},
		Entry("handleMaster failed", fmt.Errorf("master error"), nil, nil, nil, nil, nil),
		Entry("handleWorker failed", nil, fmt.Errorf("worker error"), nil, nil, nil, nil),
		Entry("handleTopology failed", nil, nil, fmt.Errorf("topology error"), nil, nil, nil),
		Entry("handleGC failed", nil, nil, nil, fmt.Errorf("gc error"), nil, nil),
		Entry("handlePrune failed", nil, nil, nil, nil, fmt.Errorf("prune error"), nil),
		Entry("handleStatus failed", nil, nil, nil, nil, nil, fmt.Errorf("status error")),
		Entry("all components succeeded", nil, nil, nil, nil, nil, nil),
	)
})