import { useState, useEffect, memo } from "react";
import { Box, Typography, CircularProgress } from "@mui/material";
import CollapsibleSection from "../../common/CollapsibleSection";
import { StyledCheckbox } from "../../../../portal/styles/authStyles";
import { teamsService } from "../../../services/teamsService";
import CustomNote from "../../common/CustomNote";
import { useParams } from "react-router-dom";

const ManageTeamsSection = memo(({
  selectedTeams,
  setSelectedTeams
}) => {
  const [teams, setTeams] = useState([]);
  const [loading, setLoading] = useState(true);
  const { id } = useParams();

  useEffect(() => {
    const fetchTeams = async () => {
      try {
        setLoading(true);
        const response = await teamsService.getTeams({ all: true });
        setTeams(response.data?.data || []);
      } catch (error) {
        console.error("Error fetching teams:", error);
      } finally {
        setLoading(false);
      }
    };

    fetchTeams();
  }, []);

  const handleTeamChange = (teamId, checked) => {
    if (checked) {
      setSelectedTeams(prev => [...prev, teamId]);
    } else {
      setSelectedTeams(prev => prev.filter(id => id !== teamId));
    }
  };

  return (
    <CollapsibleSection title="Manage Teams" defaultExpanded={id ? true : false}>
      {loading ? (
        <Box display="flex" justifyContent="center" my={2}>
          <CircularProgress size={24} />
        </Box>
      ) : teams.length > 1 || (teams.length === 1 && parseInt(teams[0].id, 10) !== 1) ? (
        <>
        <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            Teams help organize users and manage access to LLM providers, data sources, and tools. Linking teams to catalogs ensures they access only AI and data relevant to them.
        </Typography>
        <Box mt={2} ml={2}>
          <Typography variant="bodyLargeDefault" color="text.defaultSubdued">
            Select the teams this user should be part of.
          </Typography>
          <Box maxHeight="240px" overflow="auto" mt={1}>
            {teams.map(team => (
              <Box key={team.id}>
                <StyledCheckbox
                  checked={selectedTeams.includes(parseInt(team.id, 10))}
                  onChange={(checked) => handleTeamChange(parseInt(team.id, 10), checked)}
                  label={team.attributes.name}
                />
              </Box>
            ))}
          </Box>
        </Box>
        </>
      ) : (
        <CustomNote
          message="All users are automatically assigned to the default team. To add new teams, go to the Teams."
        />
      )}
    </CollapsibleSection>
  );
});

export default ManageTeamsSection;