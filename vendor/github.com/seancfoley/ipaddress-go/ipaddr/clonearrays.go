//
// Copyright 2020-2022 Sean C Foley
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package ipaddr

type addrPtrType[S any, T any] interface {
	*S
	getAddrType() addrType
	init() T
}

// returns a slice of addresses that match the same IP version as the given
func filterSeries[S any, T addrPtrType[S, T]](addr T, orig []T) []T {
	addr = addr.init()
	addrType := addr.getAddrType()
	result := append(make([]T, 0, len(orig)+1), addr)
	for _, a := range orig {
		if addrType == a.getAddrType() {
			result = append(result, a.init())
		}
	}
	return result
}

func cloneIPAddrs(sect *IPAddress, orig []*IPAddress) []ExtendedIPSegmentSeries {
	converter := func(a *IPAddress) ExtendedIPSegmentSeries { return wrapIPAddress(a) }
	if sect == nil {
		return cloneTo(orig, converter)
	}
	return cloneToExtra(sect, orig, converter)
}

func cloneIPSections(sect *IPAddressSection, orig []*IPAddressSection) []ExtendedIPSegmentSeries {
	converter := func(a *IPAddressSection) ExtendedIPSegmentSeries { return wrapIPSection(a) }
	if sect == nil {
		return cloneTo(orig, converter)
	}
	return cloneToExtra(sect, orig, converter)
}

func cloneTo[T any, U any](orig []T, conv func(T) U) []U {
	result := make([]U, len(orig))
	for i := range orig {
		result[i] = conv(orig[i])
	}
	return result
}

func cloneToExtra[T any, U any](sect T, orig []T, conv func(T) U) []U {
	origCount := len(orig)
	result := make([]U, origCount+1)
	result[origCount] = conv(sect)
	for i := range orig {
		result[i] = conv(orig[i])
	}
	return result
}
