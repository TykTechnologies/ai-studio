import React from "react";
import {
  Table,
  TableBody,
  Typography,
  TableHead,
  Box,
} from "@mui/material";
import { StyledTableCell, StyledTableRow } from "../../../styles/sharedStyles";
import CustomSelectBadge from "../../common/CustomSelectBadge";
import { roleBadgeConfigs } from "../utils/roleBadgeConfig";
import { TransferBox, HeaderBox, TableHeaderRow } from "../../common/transfer-list/styles";

const TeamMembersTable = ({ rows }) => {
  const columns = [
    {
      field: "name",
      headerName: "Name",
      width: { md: '35%', lg: '40%' },
      renderCell: (row) => (
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
            {row.name}
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
            {row.email}
          </Typography>
        </Box>
      )
    },
    {
      field: "role",
      headerName: "Role",
      width: { md: '45%', lg: '35%' },
      renderCell: (row) => (
        <CustomSelectBadge config={roleBadgeConfigs[row.role] || roleBadgeConfigs["Chat user"]} />
      )
    }
  ];

  return (
    <TransferBox sx={{ border: "none" }}>
      <HeaderBox>
        <Typography variant="bodyLargeBold" color="text.primary">
          Current members
        </Typography>
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
          Users currently on this team
        </Typography>
      </HeaderBox>
      <Table style={{ width: "100%", tableLayout: "fixed" }}>
        <TableHead>
            <TableHeaderRow>
                {columns.map((column) => (
                <StyledTableCell
                    key={column.field}
                    sx={{
                    width: column.width,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    }}
                >
                    <Typography variant="bodyMediumSemiBold" color="text.defaultSubdued">
                    {column.headerName}
                    </Typography>
                </StyledTableCell>
                ))}
            </TableHeaderRow>
        </TableHead>
        <TableBody>
          {rows.length > 0 ? (
            rows.map((row) => (
              <StyledTableRow key={row.id}>
                {columns.map((column) => (
                  <StyledTableCell
                    key={`${row.id}-${column.field}`}
                    sx={{
                      width: column.width,
                      overflow: "hidden",
                      textOverflow: "ellipsis",
                    }}
                  >
                    {column.renderCell(row)}
                  </StyledTableCell>
                ))}
              </StyledTableRow>
            ))
          ) : (
            <StyledTableRow>
              <StyledTableCell colSpan={columns.length} align="center">
                <Typography color="text.defaultSubdued">
                  No team members
                </Typography>
              </StyledTableCell>
            </StyledTableRow>
          )}
        </TableBody>
      </Table>
    </TransferBox>
  );
};

export default TeamMembersTable;