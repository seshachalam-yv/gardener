# GEP-28: Immutable ETCD Backups

## Table of Contents
- [GEP-28: Immutable ETCD Backups](#gep-28-immutable-etcd-backups)
  - [Table of Contents](#table-of-contents)
  - [Motivation](#motivation)
  - [Goal](#goal)
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
  - [Compatibility](#compatibility)
  - [Implementation Steps](#implementation-steps)
  - [Operational Considerations](#operational-considerations)

## Motivation
To enhance the reliability and integrity of ETCD backups by preventing unauthorized modifications, ensuring that backups are consistently available and intact for restoration.

## Goal
Implement immutable backup support in Gardener's ETCD clusters to secure backup data against unintended or malicious modifications post-creation.

## Proposal

### Overview
Introduce immutability in backup storage to prevent data alterations using cloud provider features that support a write-once-read-many (WORM) model.

### Detailed Design

#### Bucket Lock Mechanism
The Bucket Lock feature configures a retention policy for a Cloud Storage bucket, governing how long objects in the bucket must be retained. It also allows for the locking of the bucket's retention policy, permanently preventing the policy from being reduced or removed.

- **Supported by major providers:**
  - GCS: [Bucket Lock](https://cloud.google.com/storage/docs/bucket-lock)
  - S3: [Object Lock](https://docs.aws.amazon.com/AmazonS3/latest/userguide/object-lock.html)
  - ABS: [Immutable Blob Storage](https://learn.microsoft.com/en-us/azure/storage/blobs/immutable-policy-configure-container-scope?tabs=azure-portal)

#### Gardener Seed Backup Configuration
```yaml
backup:
  providerConfig:
    immutableSettings:
      retentionType: "Bucket"
      retentionPeriod: "96h"
```
This configuration under the `providerConfig` in the seed's backup section enables Gardener to create the required backup bucket specifications that enforce the immutability policy.

#### Gardener Extension Provider
The Gardener extension provider updates the bucket based on the `immutableSettings`:
- **If the bucket does not exist:** It is created with the immutable settings.
- **If the bucket exists without immutable settings:** The bucket is updated to include the immutable settings.
- **If the bucket has immutable settings with a shorter retention period:** The retention period is adjusted to match the new settings.

#### Admission Validation
A new admission webhook will ensure that once immutability settings are enabled:
- They cannot be disabled.
- The retention period cannot be reduced.

#### Handling of Hibernated Shoots

In scenarios where a shoot has been hibernated for a duration that exceeds the immutable retention period, the backups may become mutable again, compromising the intended immutability guarantees. To address this issue, we propose ensuring that snapshots are taken even during the hibernation of shoots, to maintain compliance with the retention policy. This process can occur during either the scheduled maintenance windows or outside these periods, as necessary.

There are two suggested approaches to manage this:

1. **Proactive Snapshotting During Reconciliation:**
   - On every reconciliation of a hibernated shoot, the ETCD is temporarily activated to take a necessary snapshot, after which it is scaled down again.
   - **Advantages:** This method ensures consistency across all cases, maintaining backup integrity regardless of the shoot's operational state.

2. **Conditional Snapshotting Based on Time Elapsed:**
   - Check the time elapsed since the last successful backup by evaluating `time.now().Since(etcd.Status.Conditions[BackupReady].LastUpdateTime)` and compare it with 50% or a predetermined fraction of the `immutableSettings.retentionPeriod`.
   - **Advantages:** This method offers a dynamic backup strategy that scales based on the actual age of the last backup, potentially conserving resources if backups are still within the safe period.

**Recommended Approach:** The first approach is recommended for its consistency and simplicity in ensuring that all backups meet the immutability criteria without introducing complex timing logic.

Additionally, to provide operational flexibility, the annotation `shoot.gardener.cloud/skip-hibernation-backup` can be used. Allows operators to skip the backup process during hibernation if set to "true".

### Lifecycle of Immutable Settings
- **Creation:** Specified upon creating a backup configuration and embedded into the backup bucket specifications.
- **Modification:** Immutable settings cannot be disabled or reduced once set.
- **Deletion:** Ensures data remains protected for the entire retention period, even if the shoot is deleted. Backup entries are removed after the `controllers.backupEntry.deletionGracePeriodHours`.

### Excluding Snapshots Under Specific Circumstances:

Given that immutable backups cannot be deleted, there are scenarios, such as corrupted snapshots or other anomalies, where certain snapshots must be skipped during the restoration process. To facilitate this:

- **Custom Metadata Tags**: Utilize custom metadata to mark specific objects (snapshots) that should be bypassed. To exclude a snapshot from the restoration process, attach custom metadata to it with the key `x-etcd-snapshot-exclude` and value `true`. This method is officially supported as demonstrated in the [etcd-backup-restore PR](https://github.com/gardener/etcd-backup-restore/pull/776).

## Rationale for a Four-Day Immutability Period
The policy maintains four days of delta snapshots and thirty days of full snapshots. A four-day immutability period ensures all objects are protected for at least this duration, balancing protection with operational flexibility.

## Compatibility
Updates will be required in the [Seed backup section](https://github.com/gardener/gardener/blob/master/pkg/apis/core/v1beta1/types_seed.go#L119-L132) of the Gardener API under `providerConfig`, ensuring backwards compatibility with existing setups where immutability is not enabled.

## Implementation Steps
1. Implement admission webhooks for validating immutability settings at the seed level.
2. Develop mechanisms to manage backup processes for hibernated shoots, considering the new immutability constraints.
