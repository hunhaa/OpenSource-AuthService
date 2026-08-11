package auth

import (
	"fmt"
	"strings"

	"github.com/Yeah114/g79client"
)

var DefaultSkinItemID = "4671864925921267367"
func GetSkinInfo(cli *g79client.Client) (SkinInfo, error) {
	userSettingList, err := cli.GetUserSettingList()
	if err != nil {
		return SkinInfo{}, fmt.Errorf("GetUserSettingList: %w", err)
	}
	if userSettingList.Code != 0 {
		return SkinInfo{}, fmt.Errorf("GetUserSettingList: %s(%d)", userSettingList.Message, userSettingList.Code)
	}
	itemID := userSettingList.Entity.SkinData.ItemID
	if itemID == "" || strings.HasPrefix(itemID, "-") {
		if DefaultSkinItemID == "" {
			return SkinInfo{}, fmt.Errorf("ChangeSkin: missing default skin id")
		}
		if err := cli.ChangeSkin(DefaultSkinItemID); err != nil {
			return SkinInfo{}, fmt.Errorf("ChangeSkin: %w", err)
		}
		itemID = DefaultSkinItemID
	}
	downloadInfo, err := cli.GetDownloadInfo(itemID)
	if err != nil {
		return SkinInfo{}, fmt.Errorf("GetDownloadInfo: %w", err)
	}
	if downloadInfo.Code != 0 {
		return SkinInfo{}, fmt.Errorf("GetDownloadInfo: %s(%d)", downloadInfo.Message, downloadInfo.Code)
	}
	return SkinInfo{
		ItemID:          itemID,
		SkinDownloadURL: downloadInfo.Entity.ResURL,
		SkinIsSlim:      true,
	}, nil
}
