---

title: Immutable ETCD Backups
gep-number: 28
creation-date: 2024-09-16
status: implementable
authors:
- "@seshachalam-yv"
- "@renormalize"
reviewers:
- "@unmarshall"
---

# GEP-28: Immutable ETCD Backups

## Table of Contents

- [GEP-28: Immutable ETCD Backups](#gep-28-immutable-etcd-backups)
  - [Summary](#summary)
  - [Motivation](#motivation)
    - [Goals](#goals)
    - [Non-Goals](#non-goals)
  - [Proposal](#proposal)
    - [Overview](#overview)
    - [Detailed Design](#detailed-design)
      - [Bucket Lock Mechanism](#bucket-lock-mechanism)
      - [Gardener Seed Backup Configuration](#gardener-seed-backup-configuration)
      - [Gardener Extension Provider](#gardener-extension-provider)
      - [Admission Validation](#admission-validation)
      - [Handling of Hibernated Shoots](#handling-of-hibernated-shoots)
      - [Lifecycle of Immutable Settings](#lifecycle-of-immutable-settings)
      - [Rationale for a Four-Day Immutability Period](#rationale-for-a-four-day-immutability-period)
      - [Excluding Snapshots Under Specific Circumstances](#excluding-snapshots-under-specific-circumstances)
  - [Compatibility](#compatibility)
  - [Implementation Steps](#implementation-steps)
  - [Risks and Mitigations](#risks-and-mitigations)
  - [Operational Considerations](#operational-considerations)
  - [Alternatives](#alternatives)
    - [Object-Level Retention Policies vs Bucket-Level Retention Policies](#object-level-retention-policies-vs-bucket-level-retention-policies)
      - [Feasibility Study: Immutable Backups on Cloud Providers](#feasibility-study-immutable-backups-on-cloud-providers)
      - [Considerations for Object-Level Retention](#considerations-for-object-level-retention)
      - [Conclusion](#conclusion)
  - [References](#references)

## Summary

This proposal aims to enhance the reliability and integrity of ETCD backups in Gardener by introducing immutable backups. By leveraging cloud provider features that support a write-once-read-many (WORM) model, we can prevent unauthorized modifications to backup data, ensuring that backups are consistently available and intact for restoration.

## Motivation

Ensuring the integrity and availability of ETCD backups is crucial for the stability and reliability of Kubernetes clusters managed by Gardener. By making backups immutable, we can protect against unintended or malicious modifications post-creation, thereby enhancing the overall security posture of the system.

### Goals

- Implement immutable backup support in Gardener's ETCD clusters.
- Secure backup data against unintended or unauthorized modifications after creation.
- Ensure backups are consistently available and intact for restoration purposes.
- Provide a configurable retention period for immutability settings.

### Non-Goals

- Altering existing backup mechanisms outside the scope of immutability features.
- Implementing immutability for backup storage solutions that do not support WORM capabilities.
- Addressing all possible operational scenarios outside the scope of ETCD backup immutability.

## Proposal

### Overview

Introduce immutability in backup storage by leveraging cloud provider features that support a write-once-read-many (WORM) model. This will prevent data alterations after backup creation, enhancing data integrity and security.

### Detailed Design

#### Bucket Lock Mechanism

The Bucket Lock feature configures a retention policy for a cloud storage bucket, governing how long objects in the bucket must be retained. It also allows for the locking of the bucket's retention policy, permanently preventing the policy from being reduced or removed.

- **Supported by Major Providers:**
  - **Google Cloud Storage (GCS):** [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - **Amazon S3 (S3):** [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - **Azure Blob Storage (ABS):** [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

#### Gardener Seed Backup Configuration

The immutable settings are specified in the seed's backup configuration under the `providerConfig` section:

```yaml
backup:
  providerConfig:
    immutableSettings:
      retentionType: "bucket"
      retentionPeriod: "96h"
```

This configuration enables Gardener to create the required backup bucket specifications that enforce the immutability policy.

#### Gardener Extension Provider

The Gardener extension provider updates the backup bucket based on the `immutableSettings`:

- **If the bucket does not exist:** It is created with the specified immutable settings.
- **If the bucket exists without immutable settings:** The bucket is updated to include the immutable settings.
- **If the bucket has immutable settings with a shorter retention period:** The retention period is adjusted to match the new settings.

#### Admission Validation

A new admission webhook will ensure that once immutability settings are enabled:

- They cannot be disabled.
- The retention period cannot be reduced.

This ensures that the immutability policy is enforced consistently and cannot be circumvented after being set.

#### Handling of Hibernated Shoots

In scenarios where a shoot has been hibernated for a duration that exceeds the immutable retention period, backups may become mutable again, compromising the intended immutability guarantees. To address this issue, we propose ensuring that snapshots are taken even during the hibernation of shoots to maintain compliance with the retention policy. This process can occur during either the scheduled maintenance windows or as necessary.

There are two suggested approaches to manage this:

1. **Proactive Snapshotting During Reconciliation:**

   - On every reconciliation of a hibernated shoot, the ETCD is temporarily activated to take a necessary snapshot, after which it is scaled down again.
   - **Advantages:** This method ensures consistency across all cases, maintaining backup integrity regardless of the shoot's operational state.

2. **Conditional Snapshotting Based on Time Elapsed:**

   - Check the time elapsed since the last successful backup by evaluating `time.Now().Since(etcd.Status.Conditions[BackupReady].LastUpdateTime)` and compare it with a predetermined fraction (e.g., 50%) of the `immutableSettings.retentionPeriod`.
   - **Advantages:** This method offers a dynamic backup strategy that scales based on the actual age of the last backup, potentially conserving resources if backups are still within the safe period.

**Recommended Approach:** The first approach is recommended for its consistency and simplicity in ensuring that all backups meet the immutability criteria without introducing complex timing logic.

To provide operational flexibility, the annotation `shoot.gardener.cloud/skip-hibernation-backup` can be used. If set to `"true"`, it allows operators to skip the backup process during hibernation.

#### Lifecycle of Immutable Settings

- **Creation:** Specified upon creating a backup configuration and embedded into the backup bucket specifications.
- **Modification:** Immutable settings cannot be disabled or reduced once set.
- **Deletion:** Data remains protected for the entire retention period, even if the shoot is deleted. Backup entries are removed after the `controllers.backupEntry.deletionGracePeriodHours`.

#### Rationale for a Four-Day Immutability Period

The policy maintains four days of delta snapshots and thirty days of full snapshots. A four-day immutability period ensures all objects are protected for at least this duration, balancing protection with operational flexibility.

#### Excluding Snapshots Under Specific Circumstances

Given that immutable backups cannot be deleted, there are scenarios, such as corrupted snapshots or other anomalies, where certain snapshots must be skipped during the restoration process. To facilitate this:

- **Custom Metadata Tags:** Utilize custom metadata to mark specific objects (snapshots) that should be bypassed. To exclude a snapshot from the restoration process, attach custom metadata to it with the key `x-etcd-snapshot-exclude` and value `true`. This method is officially supported as demonstrated in the [etcd-backup-restore PR](https://github.com/gardener/etcd-backup-restore/pull/776).

## Compatibility

Updates will be required in the [Seed backup section](https://github.com/gardener/gardener/blob/master/pkg/apis/core/v1beta1/types_seed.go#L119-L132) of the Gardener API under `providerConfig`, ensuring backward compatibility with existing setups where immutability is not enabled.

## Implementation Steps

1. Implement admission webhooks for validating immutability settings at the seed level.
2. Update the Gardener extension provider to handle immutable settings when creating or updating backup buckets.
3. Develop mechanisms to manage backup processes for hibernated shoots, considering the new immutability constraints.
4. Update documentation to reflect changes and guide operators in using the new immutability features.

## Risks and Mitigations

- **Risk:** Introducing immutability could lead to increased storage costs due to the inability to delete backups before the retention period ends.
  - **Mitigation:** Carefully configure the retention period to balance between data protection needs and storage cost considerations.

- **Risk:** Operators might unintentionally set immutability settings that cannot be modified later.
  - **Mitigation:** Provide clear documentation and warnings during configuration to ensure operators understand the implications of enabling immutability.

- **Risk:** Hibernated shoots might not receive necessary backups, potentially leading to compliance issues with the retention policy.
  - **Mitigation:** Implement the recommended proactive snapshotting approach to ensure backups are taken even during hibernation.

- **Risk:** Excluding snapshots during restoration might be misused or lead to incomplete data restoration.
  - **Mitigation:** Restrict the ability to tag snapshots for exclusion to authorized personnel and ensure proper audit logging is in place.

## Operational Considerations

- **Operator Awareness:** Operators should be aware of the immutability settings and their implications on backup storage and management.

- **Documentation:** Provide documentation to operators to handle scenarios where snapshots need to be excluded from restoration.

## Alternatives

### Object-Level Retention Policies vs Bucket-Level Retention Policies

An alternative to implementing immutability via bucket-level retention is to use object-level retention policies. Object-level retention allows for more granular control over the retention periods of individual objects within a bucket, whereas bucket-level retention applies a uniform retention period to all objects in the bucket.

#### Feasibility Study: Immutable Backups on Cloud Providers

Major cloud storage providers such as Google Cloud Storage (GCS), Amazon S3, and Azure Blob Storage (ABS) support both bucket-level and object-level retention mechanisms to enforce data immutability:

1. **Bucket-Level Retention Policies:**

   - Applies a uniform retention period to all objects within a bucket.
   - Once set, objects cannot be modified or deleted until the retention period expires.
   - Simplifies management by applying the same policy to all objects.

2. **Object-Level Retention Policies:**

   - Allows setting retention periods on a per-object basis.
   - Offers granular control, enabling different retention durations for individual objects.
   - Can accommodate varying retention requirements for different types of backups.

> **Note:** In some providers, enabling object-level retention requires bucket-level retention to be set first (e.g., in Amazon S3 and Azure Blob Storage).

Additionally, enabling object-level retention on existing buckets may not be supported or may require additional steps. For example, in Google Cloud Storage (GCS), enabling object retention lock on an already existing bucket is not currently available.[ After raising a support ticket with the Cloud Storage product team](https://issuetracker.google.com/issues/346679415?pli=1), we were informed that this feature will be generally available (GA) in early to mid Q3.

<details><summary>Comparison of Storage Provider Properties for Bucket-Level and Object-Level Immutability</summary>

| Feature                                                         | GCS | AWS | Azure |
|-----------------------------------------------------------------|-----|-----|-------|
| Can bucket-level retention period be increased?                 | Yes | Yes* | Yes (only 5 times) |
| Can bucket-level retention period be decreased?                 | No  | Yes* | No    |
| Is bucket-level retention a prerequisite for object-level retention? | No  | Yes | Yes (for existing buckets), No (for new buckets) |
| Can object-level retention period be increased?                 | Yes | Yes | Yes   |
| Can object-level retention period be decreased?                 | No  | No  | No    |
| Support for enabling object-level immutability in existing buckets | No (planned support by early to mid Q3) | Yes (only new objects will have immutability) | Yes (Azure handles the migration) |
| Support for enabling object-level immutability in new buckets   | Yes | Yes | Yes   |
| Precedence between bucket-level and object-level retention periods | Maximum of bucket or object-level retention | Object-level retention has precedence | Object-level retention has precedence |

> **Note:** *In AWS S3, changes to the bucket-level retention period can be blocked by adding a specific bucket policy.

</details>

#### Considerations for Object-Level Retention

Using object-level retention provides flexibility in scenarios where certain backups require different retention periods. For example, in the case of hibernated shoots, where the ETCD cluster may not be running and backups are not being updated, object-level retention allows extending the immutability of the latest snapshots without affecting the retention of older backups.

**Advantages:**

- **Granular Control:** Allows setting different retention periods for different objects, accommodating varying requirements.
- **Efficient Resource Utilization:** Prevents unnecessary extension of immutability for all objects, potentially reducing storage costs.
- **Enhanced Flexibility:** Can adjust retention periods for specific backups as needed.

**Disadvantages:**

- **Provider Limitations:** Not all providers support enabling object-level retention on existing buckets without additional steps. For instance, in GCS, enabling object-level retention on existing buckets is currently not supported. However, this feature is expected to become generally available in early to mid Q3, as confirmed by the Cloud Storage product team. This limitation necessitates creating new buckets or waiting for the feature to become available.

- **Prerequisite Requirements:** In some providers, object-level retention requires bucket-level retention to be set first (e.g., in Amazon S3 and Azure Blob Storage), adding complexity to the configuration.

- **Increased Complexity:** Managing retention policies at the object level requires additional logic in backup processes and tooling.


#### Conclusion

While object-level retention offers greater flexibility and control, current provider limitations and operational complexities make it less practical for immediate implementation. Specifically, the inability to enable object-level retention on existing buckets in GCS until early to mid Q3 and the prerequisite of bucket-level retention in some providers are significant factors.

Given these considerations, we propose starting with bucket-level retention to achieve immediate enhancement of backup immutability with minimal changes to existing processes. This approach allows us to implement immutability features across all providers consistently.

Once provider support for object-level retention on existing buckets improves and operational complexities are addressed, we can consider adopting object-level retention in the future to address specific requirements, such as varying retention periods for different backups.

These are the reasons why we are initially opting for bucket-level retention, with the possibility of transitioning to object-level retention when it becomes more feasible.



## References

- [GCS Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
- [AWS S3 Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
- [Azure Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)
- [etcd-backup-restore PR #776](https://github.com/gardener/etcd-backup-restore/pull/776)

---

