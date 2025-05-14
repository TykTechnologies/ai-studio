import { useState, useCallback, useEffect } from "react";
import { useNavigate } from "react-router-dom";
import { teamsService } from "../../../services/teamsService";

export const useGroupForm = (id, initialSelectedUsers = [], initialCatalogs = [], initialDataCatalogs = [], initialToolCatalogs = []) => {
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [selectedUsers, setSelectedUsers] = useState(initialSelectedUsers);
  
  const [selectedCatalogs, setSelectedCatalogs] = useState(initialCatalogs);
  const [selectedDataCatalogs, setSelectedDataCatalogs] = useState(initialDataCatalogs);
  const [selectedToolCatalogs, setSelectedToolCatalogs] = useState(initialToolCatalogs);
  
  const navigate = useNavigate();

  const fetchGroup = useCallback(async () => {
    if (!id) return;

    try {
      setLoading(true);
      const response = await teamsService.getTeam(id);
      setName(response.data.attributes.name);
      
      if (response.data.attributes.users) {
        setSelectedUsers(response.data.attributes.users);
      }
      
      if (response.data.attributes.catalogues) {
        setSelectedCatalogs(response.data.attributes.catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (response.data.attributes.data_catalogues) {
        setSelectedDataCatalogs(response.data.attributes.data_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      if (response.data.attributes.tool_catalogues) {
        setSelectedToolCatalogs(response.data.attributes.tool_catalogues.map(cat => ({
          value: cat.id,
          label: cat.attributes.name
        })));
      }
      
      setLoading(false);
    } catch (error) {
      console.error("Error fetching group", error);
      setError("Failed to fetch group");
      setLoading(false);
    }
  }, [id]);

  useEffect(() => {
    if (id) {
      fetchGroup();
    }
  }, [id, fetchGroup]);

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setError(null);

    console.log("selectedCatalogs", selectedCatalogs);
    console.log("selectedDataCatalogs", selectedDataCatalogs);
    console.log("selectedToolCatalogs", selectedToolCatalogs);

    const groupData = {
      data: {
        type: "Group",
        attributes: {
          name,
          members: selectedUsers.map(user => parseInt(user.id, 10)),
          catalogues: selectedCatalogs.map(cat => parseInt(cat.value, 10)),
          data_catalogues: selectedDataCatalogs.map(cat => parseInt(cat.value, 10)),
          tool_catalogues: selectedToolCatalogs.map(cat => parseInt(cat.value, 10))
        },
      },
    };

    console.log("Group data to save:", groupData);

    try {
      if (id) {
        await teamsService.updateTeam(id, groupData);
      } else {
        await teamsService.createTeam(groupData);
      }
      navigate("/admin/groups");
    } catch (error) {
      console.error("Error saving group", error);
      setError("Failed to save group");
      setLoading(false);
    }
  };

  return {
    name,
    setName,
    loading,
    error,
    selectedUsers,
    setSelectedUsers,
    selectedCatalogs,
    setSelectedCatalogs,
    selectedDataCatalogs,
    setSelectedDataCatalogs,
    selectedToolCatalogs,
    setSelectedToolCatalogs,
    handleSubmit
  };
};