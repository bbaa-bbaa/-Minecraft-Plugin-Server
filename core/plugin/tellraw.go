// Copyright 2024 bbaa
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package plugin

type TellrawMessage struct {
	Text       string                     `json:"text"`
	Color      string                     `json:"color,omitempty"`
	Bold       bool                       `json:"bold,omitempty"`
	HoverEvent *TellrawMessage_HoverEvent `json:"hoverEvent,omitempty"`
}

type TellrawMessage_HoverEvent struct {
	Action   string `json:"action"`
	Contents any    `json:"contents"`
}

type TellrawMessage_HoverEvent_Text TellrawMessage

type TellrawMessage_HoverEvent_Item struct {
	Item  string `json:"string"`
	Count int    `json:"count,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

type TellrawMessage_HoverEvent_Entity struct {
	Name *TellrawMessage `json:"name,omitempty"`
	Type string          `json:"type,omitempty"`
	UUID string          `json:"uuid"`
}
