import React from 'react';
import { render, screen, act } from '@testing-library/react';
import { makePluginToolRenderer } from './PluginToolRenderer';

const TAG = 'x-weather-card';

beforeAll(() => {
  if (!customElements.get(TAG)) {
    customElements.define(
      TAG,
      class extends HTMLElement {
        render() {
          this.textContent = `weather for ${this.getAttribute('data-args')}`;
        }
      },
    );
  }
});

const Wrapper = () => React.createElement(TAG);

describe('makePluginToolRenderer', () => {
  it('passes the tool call to the element and forwards tool-result events', () => {
    const Renderer = makePluginToolRenderer(TAG, Wrapper);
    const addResult = jest.fn();
    const { rerender } = render(
      <Renderer toolName="getWeather" args={{ city: 'Auckland' }} status={{ type: 'running' }} addResult={addResult} />,
    );
    const el = screen.getByTestId('plugin-tool-renderer').querySelector(TAG);
    expect(el.getAttribute('data-tool-name')).toBe('getWeather');
    expect(el.getAttribute('data-args')).toBe('{"city":"Auckland"}');
    expect(el.getAttribute('data-status')).toBe('running');
    expect(el.hasAttribute('data-result')).toBe(false);
    expect(el.toolCall.args).toEqual({ city: 'Auckland' });
    expect(el.textContent).toContain('Auckland');

    rerender(
      <Renderer toolName="getWeather" args={{ city: 'Auckland' }} result={{ temp: 18 }} status={{ type: 'complete' }} addResult={addResult} />,
    );
    expect(el.getAttribute('data-result')).toBe('{"temp":18}');
    expect(el.getAttribute('data-status')).toBe('complete');

    act(() => {
      el.dispatchEvent(new CustomEvent('tool-result', { detail: { approved: true } }));
    });
    expect(addResult).toHaveBeenCalledWith({ approved: true });

    act(() => {
      el.dispatchEvent(new CustomEvent('tool-result', { detail: { result: 'nope', isError: true } }));
    });
    expect(addResult).toHaveBeenLastCalledWith({ result: 'nope', isError: true });
  });
});
