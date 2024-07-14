FROM nginx:stable-alpine


COPY nginx.conf /etc/nginx/nginx.conf

RUN mkdir /etc/nginx/sites-available
RUN mkdir /etc/nginx/sites-enabled
COPY place.nginx /etc/nginx/sites-available/
RUN ln -s /etc/nginx/sites-available/place.nginx /etc/nginx/sites-enabled/place.nginx

RUN rm -r /etc/nginx/conf.d

COPY ./web /srv/place/web

EXPOSE 80
