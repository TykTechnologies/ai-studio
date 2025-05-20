import React from "react";
import {
  Table,
  TableBody,
  TableHead,
  TableCell,
  Typography,
} from "@mui/material";
import { StyledTableCell, StyledTableRow } from "../../../styles/sharedStyles";

const TeamMembersTable = ({ columns, rows }) => (
  <Table style={{ width: "100%", tableLayout: "fixed" }}>
    <TableHead>
      <StyledTableRow>
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
      </StyledTableRow>
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
                {column.renderCell
                  ? column.renderCell(row)
                  : row[column.field]}
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
);

export default TeamMembersTable;