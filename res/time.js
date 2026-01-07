<script>
window.addEventListener("load", () => {
   const params = new URLSearchParams(window.location.search);
   const yymmdd = (params.get("date") || "").trim();
   const B = (() => {
      const date = new Date(yymmdd);
      if (isNaN(date.getTime())) {
         return new Date("1998/09/08");
      }
      return date;
   })();

   const D  = new Date() - B;
   const A  = D / (1000 * 60 * 60 * 24 * 365.25);
   const L  = 78.4;
   const H  = (A / L) * 24;
   const HH = Math.floor(H);
   const MM = Math.floor((H - HH) * 60);

   const time = document.getElementById("time");
   time.innerHTML = `${(HH % 12 || 12).toString().padStart(2, '0')}:${MM.toString().padStart(2, '0')} ${HH >= 12 ? 'PM' : 'AM'}`;
});
</script>
