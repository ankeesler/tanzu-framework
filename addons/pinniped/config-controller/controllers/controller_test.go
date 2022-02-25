package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterapiv1beta1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Controller", func() {
	var (
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "some-namespace",
			},
		}
		cluster = &clusterapiv1beta1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: namespace.Name,
				Name:      "some-name",
			},
		}
	)

	BeforeEach(func() {
		err := k8sClient.Create(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		err := k8sClient.Delete(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
	})

	Context("Cluster is created", func() {
		BeforeEach(func() {
			err := k8sClient.Create(ctx, cluster)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Cluster may have already been deleted in Context() below.
			err := k8sClient.Delete(ctx, cluster)
			k8sErr := err.(*k8serrors.StatusError)
			if k8sErr != nil {
				clusterGR := schema.GroupResource{
					Group:    cluster.GroupVersionKind().Group,
					Resource: "clusters",
				}
				Expect(k8sErr).To(Equal(k8serrors.NewNotFound(clusterGR, cluster.Name)))
			}
		})

		It("creates a secret with identity_management_type set to none", func() {
			Eventually(func(g Gomega) {
				gotSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "some-namespace",
						Name:      "some-name",
					},
				}
				wantSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "some-namespace",
						Name:      "some-name",
						Labels: map[string]string{
							"tkg.tanzu.vmware.com/addon-name":   "pinniped",
							"tkg.tanzu.vmware.com/cluster-name": "tkg-mgmt-vc",
						},
					},
					Data: map[string][]byte{
						"values.yaml": []byte(`#@data/values
#@overlay/match-child-defaults missing_ok=True
---
identity_management_type: none
`),
					},
				}
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gotSecret), gotSecret)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(gotSecret).To(Equal(wantSecret))
			}).Should(Succeed())

			Context("Cluster is deleted", func() {
				BeforeEach(func() {
					err := k8sClient.Delete(ctx, cluster)
					Expect(err).NotTo(HaveOccurred())
				})

				It("creates a secret with identity_management_type set to none", func() {
					Eventually(func(g Gomega) {
						gotSecret := &corev1.Secret{
							ObjectMeta: metav1.ObjectMeta{
								Namespace: "some-namespace",
								Name:      "some-name",
							},
						}
						err := k8sClient.Get(ctx, client.ObjectKeyFromObject(gotSecret), gotSecret)
						k8sErr := err.(*k8serrors.StatusError)
						secretGR := schema.GroupResource{
							Group:    gotSecret.GroupVersionKind().Group,
							Resource: "secrets",
						}
						g.Expect(k8sErr).To(Equal(k8serrors.NewNotFound(secretGR, gotSecret.Name)))
					}).Should(Succeed())
				})
			})
		})
	})
})
