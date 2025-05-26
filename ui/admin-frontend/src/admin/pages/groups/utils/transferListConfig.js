import { Box, Typography } from "@mui/material";
import CustomSelectBadge from "../../../components/common/CustomSelectBadge";
import { roleBadgeConfigs } from "../../../components/groups/utils/roleBadgeConfig";

export const TEAM_MEMBERS_TRANSFER_LIST_COLUMNS = [
  {
    field: "name",
    headerName: "Name",
    width: { md: '35%', lg: '40%' },
    renderCell: (item) => (
      <Box sx={{
        display: 'flex',
        flexDirection: 'column',
        width: '100%',
        pr: 1
      }}>
        <Typography
          variant="bodyMediumMedium"
          color="text.defaultSubdued"
          sx={{
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            width: '100%'
          }}
        >
          {item.attributes?.name}
        </Typography>
        <Typography
          variant="bodySmallDefault"
          color="text.defaultSubdued"
          sx={{
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            width: '100%'
          }}
        >
          {item.attributes?.email}
        </Typography>
      </Box>
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