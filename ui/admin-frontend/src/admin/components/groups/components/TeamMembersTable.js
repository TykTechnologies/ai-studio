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

const TeamMembersTable = ({ rows, columns }) => {
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