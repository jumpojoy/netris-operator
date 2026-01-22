/*
Copyright 2021. Netris, Inc.

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

package netrisstorage

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/netrisai/netriswebapi/v2/types/ipam"
)

// SubnetsStorage .
type SubnetsStorage struct {
	sync.Mutex
	Subnets    []*ipam.IPAM
	VPCStorage *VPCStorage
}

// NewSubnetsStorage .
func NewSubnetsStorage() *SubnetsStorage {
	return &SubnetsStorage{}
}

// SetVPCStorage sets the VPC storage reference.
func (p *SubnetsStorage) SetVPCStorage(vpcStorage *VPCStorage) {
	p.VPCStorage = vpcStorage
}

// GetAll .
func (p *SubnetsStorage) GetAll() []*ipam.IPAM {
	p.Lock()
	defer p.Unlock()
	return p.getAll()
}

func (p *SubnetsStorage) getAll() []*ipam.IPAM {
	subnets := []*ipam.IPAM{}
	subnets = append(subnets, p.Subnets...)
	return subnets
}

func (p *SubnetsStorage) storeAll(items []*ipam.IPAM) {
	p.Subnets = items
}

// FindByName .
func (p *SubnetsStorage) FindByName(name string) (*ipam.IPAM, bool) {
	p.Lock()
	defer p.Unlock()
	return p.findByName(name)
}

func (p *SubnetsStorage) findByName(name string) (*ipam.IPAM, bool) {
	for _, item := range p.Subnets {
		if item.Name == name {
			return item, true
		}
		if s, ok := p.findByNameInChildren(item, name); ok {
			return s, true
		}
	}
	return nil, false
}

func (p *SubnetsStorage) findByNameInChildren(ipam *ipam.IPAM, name string) (*ipam.IPAM, bool) {
	for _, item := range ipam.Children {
		if item.Name == name {
			return item, true
		}
		if s, ok := p.findByNameInChildren(item, name); ok {
			return s, true
		}
	}
	return nil, false
}

// FindByID .
func (p *SubnetsStorage) FindByID(id int, typo string) (*ipam.IPAM, bool) {
	p.Lock()
	defer p.Unlock()
	item, ok := p.findByID(id, typo)
	if !ok {
		_ = p.download()
		item, ok = p.findByID(id, typo)
		if !ok {
			// Print all items names in the storage
			allNames := p.getAllNames()
			fmt.Printf("SubnetsStorage: Item with ID=%d, Type=%s not found. Available items: %v\n", id, typo, allNames)
		}
		return item, ok
	}
	return item, ok
}

func (p *SubnetsStorage) findInChildren(ipam *ipam.IPAM, id int, typo string) (*ipam.IPAM, bool) {
	for _, item := range ipam.Children {
		if item.ID == id && item.Type == typo {
			return item, true
		}

		if s, ok := p.findInChildren(item, id, typo); ok {
			return s, true
		}
	}
	return nil, false
}

func (p *SubnetsStorage) findByID(id int, typo string) (*ipam.IPAM, bool) {
	for _, item := range p.Subnets {
		if item.ID == id && item.Type == typo {
			return item, true
		}
		if s, ok := p.findInChildren(item, id, typo); ok {
			return s, true
		}
	}
	return nil, false
}

func (p *SubnetsStorage) getAllNames() []string {
	names := []string{}
	for _, item := range p.Subnets {
		names = append(names, p.collectNames(item)...)
	}
	return names
}

func (p *SubnetsStorage) collectNames(ipam *ipam.IPAM) []string {
	names := []string{ipam.Name}
	for _, child := range ipam.Children {
		names = append(names, p.collectNames(child)...)
	}
	return names
}

// Download .
func (p *SubnetsStorage) download() error {
	// Get all VPC IDs from VPCStorage and format as comma-separated string
	filterByVpc := ""
	if p.VPCStorage != nil {
		existingVPCs := p.VPCStorage.GetAll()
		if len(existingVPCs) > 0 {
			vpcIDs := make([]string, 0, len(existingVPCs))
			for _, v := range existingVPCs {
				vpcIDs = append(vpcIDs, strconv.Itoa(v.ID))
			}
			filterByVpc = strings.Join(vpcIDs, ",")
		}
	}

	// Call Get() with filterByVpc parameter if available
	// filterByVpc is comma-separated string of VPC IDs like "1,2,3"
	var items []*ipam.IPAM
	var err error
	if filterByVpc != "" {
		items, err = Cred.IPAM().Get(filterByVpc)
	} else {
		items, err = Cred.IPAM().Get()
	}

	if err != nil {
		return err
	}
	p.storeAll(items)
	return nil
}

// Download .
func (p *SubnetsStorage) Download() error {
	p.Lock()
	defer p.Unlock()
	return p.download()
}
