#!/usr/bin/env bash
#
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o nounset
set -o pipefail
set -o errexit

source $(dirname "${0}")/ci-common.sh

clamp_mss_to_pmtu

# test setup
make kind-up

# export all container logs and events after test execution
trap "
  ( rm -rf dev/gardener; export_logs 'gardener-local';
    export_events_for_kind 'gardener-local'; export_events_for_shoots )
  ( make kind-down;)
" EXIT
export GARDENER_PREVIOUS_RELEASE=v1.60.0
export GARDENER_CURRENT_RELEASE=v1.60.1

# download gardener previous release to perform gardener upgrade tests
# $(dirname "${0}")/download_gardener_source_code.sh --gardener-version $GARDENER_PREVIOUS_RELEASE --download-path $DOWNLOAD_PATH
cd dev/$GARDENER_PREVIOUS_RELEASE/gardener
 
cp $KUBECONFIG example/provider-local/seed-kind/base/kubeconfig
cp $KUBECONFIG example/gardener-local/kind/local/kubeconfig

# install previous gardener release
echo "Installing gardener version $GARDENER_PREVIOUS_RELEASE"
make gardener-up

cd -
# run gardener pre-upgrade tests
echo "Running gardener pre-upgrade tests"
make test-gardener-pre-upgrade

cd dev/$GARDENER_CURRENT_RELEASE/gardener
 
cp $KUBECONFIG example/provider-local/seed-kind/base/kubeconfig
cp $KUBECONFIG example/gardener-local/kind/local/kubeconfig

echo "Upgrading gardener version to $GARDENER_CURRENT_RELEASE"
make gardener-up

echo "Wait until seed gets upgraded from version $GARDENER_PREVIOUS_RELEASE to $GARDENER_CURRENT_RELEASE"
kubectl wait seed local --timeout=5m \
    --for=jsonpath='{.status.gardener.version}'=$GARDENER_CURRENT_RELEASE \
    --for=condition=gardenletready --for=condition=extensionsready \
    --for=condition=bootstrapped 

cd -
echo "Running gardener post-upgrade tests"
make test-gardener-post-upgrade GARDENER_CURRENT_RELEASE=$GARDENER_CURRENT_RELEASE

make gardener-down