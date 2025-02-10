import React, { useRef, useEffect } from 'react';
import { Line, Bar } from 'react-chartjs-2';

const arePropsEqual = (prevProps, nextProps) => {
  if (!prevProps.data || !nextProps.data) return true;
  return JSON.stringify(prevProps.data.datasets[0].data) === JSON.stringify(nextProps.data.datasets[0].data);
};

export const MemoizedLineChart = React.memo(({ options, data }) => {
  const chartRef = useRef(null);

  useEffect(() => {
    const chart = chartRef.current;
    if (chart) {
      chart.update('none');
    }
  }, [options]);

  return <Line ref={chartRef} options={options} data={data} />;
}, arePropsEqual);

export const MemoizedBarChart = React.memo(({ options, data }) => {
  const chartRef = useRef(null);

  useEffect(() => {
    const chart = chartRef.current;
    if (chart) {
      chart.update('none');
    }
  }, [options]);

  return <Bar ref={chartRef} options={options} data={data} />;
}, arePropsEqual);

MemoizedLineChart.displayName = 'MemoizedLineChart';
MemoizedBarChart.displayName = 'MemoizedBarChart';
