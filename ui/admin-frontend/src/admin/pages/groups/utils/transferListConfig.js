import CustomSelectBadge from "../../../components/common/CustomSelectBadge";
import { roleBadgeConfigs } from "../../../components/groups/utils/roleBadgeConfig";
import { MemberInfoContainer, TruncatedTypography } from "../../../components/common/styles";

export const TEAM_MEMBERS_TRANSFER_LIST_COLUMNS = [
  {
    field: "name",
    headerName: "Name",
    width: { md: '35%', lg: '40%' },
    renderCell: (item) => (
      <MemberInfoContainer>
        <TruncatedTypography
          variant="bodyMediumMedium"
          color="text.defaultSubdued"
        >
          {item.attributes?.name}
        </TruncatedTypography>
        <TruncatedTypography
          variant="bodySmallDefault"
          color="text.defaultSubdued"
        >
          {item.attributes?.email}
        </TruncatedTypography>
      </MemberInfoContainer>
    )
  },
  {
    field: "role",
    headerName: "Role",
    width: { md: '45%', lg: '35%' },
    renderCell: (item) => (
      <CustomSelectBadge config={roleBadgeConfigs[item.attributes?.role] || roleBadgeConfigs["Chat user"]} />
    )
  }
]; 