import { safeImageSrc, safeBackground } from './generativeSafety';

describe('generativeSafety', () => {
  it('allows http(s), same-origin paths and inline images', () => {
    expect(safeImageSrc('https://example.com/a.png')).toBe('https://example.com/a.png');
    expect(safeImageSrc('http://example.com/a.png')).toBe('http://example.com/a.png');
    expect(safeImageSrc('/api/v1/branding/logo')).toBe('/api/v1/branding/logo');
    expect(safeImageSrc('data:image/png;base64,iVBORw0KGgo=')).toBe('data:image/png;base64,iVBORw0KGgo=');
  });

  it('rejects script, protocol-relative and other schemes', () => {
    expect(safeImageSrc('javascript:alert(1)')).toBeUndefined();
    expect(safeImageSrc('JAVASCRIPT:alert(1)')).toBeUndefined();
    expect(safeImageSrc('data:text/html;base64,PHNjcmlwdD4=')).toBeUndefined();
    expect(safeImageSrc('//evil.example/a.png')).toBeUndefined();
    expect(safeImageSrc('file:///etc/passwd')).toBeUndefined();
    expect(safeImageSrc('')).toBeUndefined();
    expect(safeImageSrc(42)).toBeUndefined();
  });

  it('allows colours and gradients as backgrounds', () => {
    expect(safeBackground('#1f2430')).toBe('#1f2430');
    expect(safeBackground('rgba(0, 0, 0, 0.5)')).toBe('rgba(0, 0, 0, 0.5)');
    expect(safeBackground('linear-gradient(135deg, #23E2C2 0%, #5900CB 100%)')).toBe('linear-gradient(135deg, #23E2C2 0%, #5900CB 100%)');
  });

  it('rejects backgrounds that fetch or evaluate', () => {
    expect(safeBackground('url(https://evil.example/x.png)')).toBeUndefined();
    expect(safeBackground('url("javascript:alert(1)")')).toBeUndefined();
    expect(safeBackground('image-set("a.png" 1x)')).toBeUndefined();
    expect(safeBackground('expression(alert(1))')).toBeUndefined();
    expect(safeBackground('red; behavior: url(x)')).toBeUndefined();
    expect(safeBackground('a'.repeat(400))).toBeUndefined();
  });
});
