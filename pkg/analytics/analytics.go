//  Copyright 2016 Red Hat, Inc.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.

package analytics

import (
	"sync"

	ga "github.com/jpillora/go-ogle-analytics"
)

const (
	id       = "UA-85532162-2"
	category = "keycloak-operator"
)

var (
	client *ga.Client
	once   sync.Once
)

func send(e *ga.Event) {
	mustClient().Send(e)
}

func mustClient() *ga.Client {
	once.Do(func() {
		c, err := ga.NewClient(id)
		if err != nil {
			panic(err)
		}
		client = c
	})
	return client
}

func KeycloakCreated() {
	send(ga.NewEvent(category, "keycloak_created"))
}

func KeycloakDeleted() {
	send(ga.NewEvent(category, "keycloak_deleted"))
}
