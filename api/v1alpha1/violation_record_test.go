package v1alpha1

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// func makeRecord(i int) v1alpha1.ViolationRecord {
// 	return v1alpha1.ViolationRecord{
// 		Timestamp:      metav1.NewTime(time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC)),
// 		PodName:        fmt.Sprintf("pod-%d", i),
// 		ContainerName:  "c",
// 		ExecutablePath: "/usr/bin/test",
// 		NodeName:       "node-1",
// 		Action:         "monitor",
// 	}
// }

func (r ViolationRecord) withID(id int64) ViolationRecord {
	r.ID = id
	return r
}

func (r ViolationRecord) withExecutable(exec string) ViolationRecord {
	r.ExecutablePath = exec
	return r
}

func (r ViolationRecord) withTimestamp(ts time.Time) ViolationRecord {
	r.Timestamp = metav1.NewTime(ts)
	return r
}

func TestMergeScrapedViolations(t *testing.T) {
	baseTS := metav1.NewTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	baseViolation := ViolationRecord{
		ID:             0,
		Timestamp:      baseTS,
		PodName:        "pod-a",
		ContainerName:  "c",
		ExecutablePath: "/x",
		NodeName:       "node-1",
		Action:         "monitor",
	}

	baseStatus := WorkloadPolicyStatus{
		Violations:     []ViolationRecord{baseViolation},
		ViolationCount: 1,
	}

	tests := []struct {
		name           string
		scraped        []ViolationRecord
		initialStatus  WorkloadPolicyStatus
		expectedStatus WorkloadPolicyStatus
		// ...
		existing   []ViolationRecord
		count      int64
		wantCount  int64
		wantMerged []ViolationRecord
		check      func(t *testing.T, merged []ViolationRecord)
	}{
		{
			name:           "no_scraped_violations",
			scraped:        nil,
			initialStatus:  baseStatus,
			expectedStatus: baseStatus,
		},
		{
			name: "scrape_new_violations",
			scraped: []ViolationRecord{
				baseViolation.withExecutable("/y").withTimestamp(baseTS.Add(time.Minute)),
				baseViolation.withExecutable("/z").withTimestamp(baseTS.Add(time.Minute * 2)),
			},
			initialStatus: baseStatus,
			expectedStatus: WorkloadPolicyStatus{
				Violations: []ViolationRecord{
					baseViolation.withExecutable("/z").withID(2).withTimestamp(baseTS.Add(time.Minute * 2)),
					baseViolation.withExecutable("/y").withID(1).withTimestamp(baseTS.Add(time.Minute)),
					baseViolation.withExecutable("/x").withID(0),
				},
				ViolationCount: 3,
			},
		},
		{
			name: "scrape_new_and_old_violations",
			scraped: []ViolationRecord{
				// New timestamp but same violation
				baseViolation.withTimestamp(baseTS.Add(time.Hour)),
				baseViolation.withExecutable("/z").withTimestamp(baseTS.Add(time.Minute)),
			},
			initialStatus: baseStatus,
			expectedStatus: WorkloadPolicyStatus{
				Violations: []ViolationRecord{
					baseViolation.withExecutable("/x").withID(0).withTimestamp(baseTS.Add(time.Hour)),
					baseViolation.withExecutable("/z").withID(2).withTimestamp(baseTS.Add(time.Minute)),
				},
				// Even if we have just 2 violations in the array, we have seen 3 of them.
				ViolationCount: 3,
			},
		},
		{
			name: "trim_to_MaxViolationRecords",
			scraped: []ViolationRecord{
				baseViolation.withExecutable("/101").
					withID(101).
					withTimestamp(baseTS.Add(time.Duration(101) * time.Minute)),
			},
			initialStatus: func() WorkloadPolicyStatus {
				r := make([]ViolationRecord, MaxViolationRecords)
				for i := range r {
					r[i] = baseViolation.withExecutable(fmt.Sprintf("/%d", i+1)).
						withID(int64(i)).
						withTimestamp(baseTS.Add(time.Duration(i+1) * time.Minute))
				}
				return WorkloadPolicyStatus{
					Violations:     r,
					ViolationCount: 100,
				}
			}(),
			expectedStatus: func() WorkloadPolicyStatus {
				r := make([]ViolationRecord, MaxViolationRecords+1)
				for i := range r {
					r[i] = baseViolation.withExecutable(fmt.Sprintf("/%d", i+1)).
						withID(int64(i)).
						withTimestamp(baseTS.Add(time.Duration(i+1) * time.Minute))
				}
				slices.SortStableFunc(r, func(a, b ViolationRecord) int {
					return b.Timestamp.Time.Compare(a.Timestamp.Time)
				})
				return WorkloadPolicyStatus{
					Violations:     r[:MaxViolationRecords],
					ViolationCount: 101,
				}
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.initialStatus.mergeScrapedViolations(tt.scraped)
			require.Equal(t, tt.expectedStatus, tt.initialStatus)
		})
	}
}

// todo!: to rework...
// func TestProcessAcknowledgement(t *testing.T) {
// 	const prefix = v1alpha1.ViolationAcknowledgePrefix
// 	now := metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

// 	tests := []struct {
// 		name                       string
// 		annotations                map[string]string
// 		violations                 []v1alpha1.ViolationRecord
// 		acknowledgedViolations     []v1alpha1.AcknowledgedViolationRecord
// 		wantViolations             []v1alpha1.ViolationRecord
// 		wantAcknowledgedViolations []v1alpha1.AcknowledgedViolationRecord
// 		wantAnnotations            map[string]string
// 		wantErr                    bool
// 	}{
// 		{
// 			name:                       "no annotations leaves status unchanged",
// 			annotations:                map[string]string{},
// 			violations:                 []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantViolations:             []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantAcknowledgedViolations: nil,
// 			wantAnnotations:            map[string]string{},
// 		},
// 		{
// 			name:           "single annotation matches violation",
// 			annotations:    map[string]string{prefix + "1": "looks intentional"},
// 			violations:     []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantViolations: []v1alpha1.ViolationRecord{},
// 			wantAcknowledgedViolations: []v1alpha1.AcknowledgedViolationRecord{{
// 				Violation: withID(makeRecord(1), 1),
// 				Reason:    "looks intentional",
// 			}},
// 			wantAnnotations: map[string]string{},
// 		},
// 		{
// 			name: "multiple annotations match multiple violations",
// 			annotations: map[string]string{
// 				prefix + "1": "reason one",
// 				prefix + "2": "reason two",
// 			},
// 			violations:     []v1alpha1.ViolationRecord{withID(makeRecord(1), 1), withID(makeRecord(2), 2)},
// 			wantViolations: []v1alpha1.ViolationRecord{},
// 			wantAcknowledgedViolations: []v1alpha1.AcknowledgedViolationRecord{
// 				{Violation: withID(makeRecord(2), 2), Reason: "reason two"},
// 				{Violation: withID(makeRecord(1), 1), Reason: "reason one"},
// 			},
// 			wantAnnotations: map[string]string{},
// 		},
// 		{
// 			name: "partial match leaves unacknowledged violation in place",
// 			annotations: map[string]string{
// 				prefix + "1":  "acknowledged",
// 				prefix + "99": "no match",
// 			},
// 			violations:     []v1alpha1.ViolationRecord{withID(makeRecord(1), 1), withID(makeRecord(2), 2)},
// 			wantViolations: []v1alpha1.ViolationRecord{withID(makeRecord(2), 2)},
// 			wantAcknowledgedViolations: []v1alpha1.AcknowledgedViolationRecord{{
// 				Violation: withID(makeRecord(1), 1),
// 				Reason:    "acknowledged",
// 			}},
// 			wantAnnotations: map[string]string{},
// 		},
// 		{
// 			name:                       "no matching violation for annotation: key deleted, violation kept",
// 			annotations:                map[string]string{prefix + "999": "unmatched"},
// 			violations:                 []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantViolations:             []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantAcknowledgedViolations: []v1alpha1.AcknowledgedViolationRecord{},
// 			wantAnnotations:            map[string]string{},
// 		},
// 		{
// 			name:                       "malformed id returns error and leaves status unchanged",
// 			annotations:                map[string]string{prefix + "not-a-number": "reason"},
// 			violations:                 []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantViolations:             []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantAcknowledgedViolations: nil,
// 			wantAnnotations:            map[string]string{prefix + "not-a-number": "reason"},
// 			wantErr:                    true,
// 		},
// 		{
// 			name:                       "unrelated annotation is preserved",
// 			annotations:                map[string]string{"unrelated.io/key": "value"},
// 			violations:                 []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantViolations:             []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			wantAcknowledgedViolations: nil,
// 			wantAnnotations:            map[string]string{"unrelated.io/key": "value"},
// 		},
// 		{
// 			name:        "trims acknowledged violations to MaxViolationRecords",
// 			annotations: map[string]string{prefix + "1": "new ack"},
// 			violations:  []v1alpha1.ViolationRecord{withID(makeRecord(1), 1)},
// 			acknowledgedViolations: func() []v1alpha1.AcknowledgedViolationRecord {
// 				result := make([]v1alpha1.AcknowledgedViolationRecord, v1alpha1.MaxViolationRecords)
// 				for i := range result {
// 					result[i] = v1alpha1.AcknowledgedViolationRecord{
// 						Violation: withID(makeRecord(i+10), int64(i+10)),
// 						Reason:    "pre-existing",
// 					}
// 				}
// 				return result
// 			}(),
// 			wantViolations: []v1alpha1.ViolationRecord{},
// 			wantAcknowledgedViolations: func() []v1alpha1.AcknowledgedViolationRecord {
// 				result := make([]v1alpha1.AcknowledgedViolationRecord, v1alpha1.MaxViolationRecords)
// 				result[0] = v1alpha1.AcknowledgedViolationRecord{
// 					Violation: withID(makeRecord(1), 1),
// 					Reason:    "new ack",
// 				}
// 				for i := range v1alpha1.MaxViolationRecords - 1 {
// 					result[i+1] = v1alpha1.AcknowledgedViolationRecord{
// 						Violation: withID(makeRecord(i+10), int64(i+10)),
// 						Reason:    "pre-existing",
// 					}
// 				}
// 				return result
// 			}(),
// 			wantAnnotations: map[string]string{},
// 		},
// 		{
// 			name:                       "empty violations with annotation: acknowledged list stays empty",
// 			annotations:                map[string]string{prefix + "42": "reason"},
// 			violations:                 nil,
// 			wantViolations:             nil,
// 			wantAcknowledgedViolations: []v1alpha1.AcknowledgedViolationRecord{},
// 			wantAnnotations:            map[string]string{},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			wp := &v1alpha1.WorkloadPolicy{ObjectMeta: metav1.ObjectMeta{Annotations: tt.annotations}}
// 			wp.Status = v1alpha1.WorkloadPolicyStatus{
// 				Violations:             tt.violations,
// 				AcknowledgedViolations: tt.acknowledgedViolations,
// 			}

// 			err := wp.ProcessPolicyStatus([]v1alpha1.PolicyNodeStatus{}, nil, now)
// 			if tt.wantErr {
// 				require.Error(t, err)
// 			} else {
// 				require.NoError(t, err)
// 			}

// 			for i := range wp.Status.AcknowledgedViolations {
// 				wp.Status.AcknowledgedViolations[i].AcknowledgedAt = metav1.Time{}
// 			}

// 			require.Equal(t, tt.wantViolations, wp.Status.Violations)
// 			require.Equal(t, tt.wantAcknowledgedViolations, wp.Status.AcknowledgedViolations)
// 			require.Equal(t, tt.wantAnnotations, wp.Annotations)
// 		})
// 	}
// }
