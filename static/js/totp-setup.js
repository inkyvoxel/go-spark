(function () {
  var el = document.getElementById("totp-qr");
  if (!el) return;
  var uri = el.getAttribute("data-otpauth-uri");
  if (!uri) return;
  new QRCode(el, { text: uri, width: 200, height: 200 });
})();
