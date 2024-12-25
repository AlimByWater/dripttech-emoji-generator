package types

type Permissions struct {
	UserID                int64   `json:"user_id" db:"user_id"`
	PrivateGeneration     bool    `json:"private_generation" db:"private_generation"`
	PackNameWithoutPrefix bool    `json:"pack_name_without_prefix" db:"pack_name_without_prefix"`
	UseInGroups           bool    `json:"use_in_groups" db:"use_in_groups"`
	UseByChannelName      bool    `json:"use_by_channel_name" db:"use_by_channel_name"`
	ChannelIDs            []int64 `json:"channel_ids" db:"channel_ids"`
}
