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

package tellraw

type Color string

type MsgType string

var (
	Black        Color = "black"
	Dark_Blue    Color = "dark_blue"
	Dark_Green   Color = "dark_green"
	Dark_Aqua    Color = "dark_aqua"
	Dark_Red     Color = "dark_red"
	Dark_Purple  Color = "dark_purple"
	Bold         Color = "gold"
	Gray         Color = "gray"
	Dark_Gray    Color = "dark_gray"
	Blue         Color = "blue"
	Green        Color = "green"
	Aqua         Color = "aqua"
	Red          Color = "red"
	Light_Purple Color = "light_purple"
	Yellow       Color = "yellow"
	White        Color = "white"
)

var (
	Text         MsgType = "text"
	Translatable MsgType = "translatable"
	Score        MsgType = "score"
	Nbt          MsgType = "nbt"
	Selector     MsgType = "selector"
	Keybind      MsgType = "keybind"
)

type Message struct {
	Text          string      `json:"text"`
	Color         Color       `json:"color,omitempty"`
	Type          MsgType     `json:"type,omitempty"`
	Insertion     string      `json:"insertion,omitempty"`
	Font          string      `json:"font,omitempty"`
	Selector      string      `json:"selector,omitempty"`
	Separator     *Message    `json:"separator,omitempty"`
	Bold          bool        `json:"bold,omitempty"`
	Italic        bool        `json:"italic,omitempty"`
	Underlined    bool        `json:"underlined,omitempty"`
	Strikethrough bool        `json:"strikethrough,omitempty"`
	Obfuscated    bool        `json:"obfuscated,omitempty"`
	HoverEvent    *HoverEvent `json:"hoverEvent,omitempty"`
	ClickEvent    *ClickEvent `json:"clickEvent,omitempty"`
}

type HoverEvent_Action string

var (
	HoverEvent_Show_Text   HoverEvent_Action = "show_text"
	HoverEvent_Show_Item   HoverEvent_Action = "show_item"
	HoverEvent_Show_Entity HoverEvent_Action = "show_entity"
)

type HoverEvent struct {
	Action   HoverEvent_Action `json:"action"`
	Contents any               `json:"contents"`
}

type HoverEvent_Item struct {
	Item  string `json:"string"`
	Count int    `json:"count,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

type HoverEvent_Entity struct {
	Name *Message `json:"name,omitempty"`
	Type string   `json:"type,omitempty"`
	UUID string   `json:"uuid"`
}

type ClickEvent struct {
	Action string `json:"action"`
	Value  string `json:"value"`
	GoFunc func() `json:"-"`
}
