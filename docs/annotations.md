# Annotations

This document provides a list of annotations used by Hive CRDs


| Annotation| Description | 
| ---------- | ----------- |
| hive.openshift.io/syncset-pause | When the value is "true", Hive will stop syncing everything to target cluster including resources defined in `syncset` object, and remote machineset.  | 