// Chart options and dataset builders for the token / cost charts shown on the
// LLM details page and the per-model detail page. Both pages render the
// MultiAxisChartData returned by GET /analytics/usage, so they share this so
// the two views cannot drift apart.

export const tokenChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  scales: {
    x: {
      type: "time",
      time: {
        unit: "day",
      },
      title: {
        display: true,
        text: "Date",
      },
      stacked: true,
    },
    y: {
      beginAtZero: true,
      title: {
        display: true,
        text: "Token Usage",
      },
      stacked: true,
    },
  },
  plugins: {
    legend: {
      position: "top",
    },
    title: {
      display: true,
      text: "Token Usage Over Time",
    },
    tooltip: {
      mode: "index",
    },
  },
};

export const costChartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  scales: {
    x: {
      type: "time",
      time: {
        unit: "day",
      },
      title: {
        display: true,
        text: "Date",
      },
    },
    y: {
      beginAtZero: true,
      title: {
        display: true,
        text: "Cost ($)",
      },
    },
  },
  plugins: {
    legend: {
      position: "top",
    },
    title: {
      display: true,
      text: "Cost Over Time",
    },
  },
};

// Dataset indexes in MultiAxisChartData from /analytics/usage:
// 0 total tokens, 1 cost, 2 prompt, 3 response, 4 cache write, 5 cache read.
export const buildTokenChartData = (usage) => ({
  labels: usage?.labels || [],
  datasets: [
    {
      label: "Prompt Tokens",
      data: usage?.datasets?.[2]?.data || [],
      borderColor: "rgb(53, 162, 235)",
      backgroundColor: "rgba(53, 162, 235, 0.5)",
      fill: true,
    },
    {
      label: "Response Tokens",
      data: usage?.datasets?.[3]?.data || [],
      borderColor: "rgb(75, 192, 192)",
      backgroundColor: "rgba(75, 192, 192, 0.5)",
      fill: true,
    },
    {
      label: "Cache Write Tokens",
      data: usage?.datasets?.[4]?.data || [],
      borderColor: "rgb(255, 159, 64)",
      backgroundColor: "rgba(255, 159, 64, 0.5)",
      fill: true,
    },
    {
      label: "Cache Read Tokens",
      data: usage?.datasets?.[5]?.data || [],
      borderColor: "rgb(153, 102, 255)",
      backgroundColor: "rgba(153, 102, 255, 0.5)",
      fill: true,
    },
  ],
});

// The API tags the cost series with yAxisID "y1" for the multi-axis app view.
// Only the data is taken here: copying the axis id would make Chart.js add a
// second, empty y axis beside the configured "Cost ($)" one.
export const buildCostChartData = (usage) => ({
  labels: usage?.labels || [],
  datasets: [
    {
      label: usage?.datasets?.[1]?.label || "Cost",
      data: usage?.datasets?.[1]?.data || [],
      borderColor: "rgb(255, 99, 132)",
      backgroundColor: "rgba(255, 99, 132, 0.5)",
      tension: 0.1,
    },
  ],
});
