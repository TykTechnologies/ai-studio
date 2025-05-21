import React, { memo } from "react";
import {
  Table,
  TableBody,
  Typography,
  TableHead,
  Box,
  CircularProgress,
} from "@mui/material";
import { StyledTableCell, StyledTableRow } from "../../../styles/sharedStyles";
import { TransferBox, HeaderBox, TableHeaderRow } from "../../common/transfer-list/styles";

const TeamMembersTable = memo(({ 
  rows, 
  columns, 
  loading, 
  isLoadingMore, 
  containerRef 
}) => {

  return (
    <TransferBox 
      ref={containerRef} 
      sx={{ 
        border: "none", 
      }}
    >
      <HeaderBox>
        <Typography variant="bodyLargeBold" color="text.primary">
          Current members
        </Typography>
        <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
          Users currently on this team
        </Typography>
      </HeaderBox>
      <Table 
        style={{ 
          width: "100%", 
          tableLayout: "fixed" 
        }}
      >
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
          {loading && rows?.length === 0 ? (
            <StyledTableRow>
              <StyledTableCell colSpan={columns.length} align="center">
                <CircularProgress size={24} />
              </StyledTableCell>
            </StyledTableRow>
          ) : rows?.length > 0 ? (
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
      
      {isLoadingMore && (
        <Box display="flex" justifyContent="center" p={2}>
          <Typography variant="bodyMediumDefault" color="text.defaultSubdued">
            Loading more users...
          </Typography>
        </Box>
      )}
    </TransferBox>
  );
});

export default TeamMembersTable;